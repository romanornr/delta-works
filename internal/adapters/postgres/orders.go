package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/romanornr/delta-works/internal/adapters/postgres/sqlcgen"
	"github.com/romanornr/delta-works/internal/domain/instrument"
	"github.com/romanornr/delta-works/internal/domain/money"
	"github.com/romanornr/delta-works/internal/domain/order"
	"github.com/romanornr/delta-works/internal/ports"
)

// OrderStore persists orders per the M2 state machine. ApplyEvent is the
// single write path for venue events: transition row, fill row and outbox
// rows commit atomically (docs/specs/m2-oms.md, ADR-0008).
type OrderStore struct {
	pool *pgxpool.Pool
	q    *sqlcgen.Queries
}

var _ ports.OrderStore = (*OrderStore)(nil)

// NewOrderStore returns an OrderStore backed by pool.
func NewOrderStore(pool *pgxpool.Pool) *OrderStore {
	return &OrderStore{pool: pool, q: sqlcgen.New(pool)}
}

// CreatePending inserts the pending row before the venue submit.
// Re-inserting the same ClientOrderID is a no-op, so submit retries with
// the same ULID are safe.
func (s *OrderStore) CreatePending(ctx context.Context, req order.Request) error {
	_, err := s.q.InsertPendingOrder(ctx, sqlcgen.InsertPendingOrderParams{
		ClientOrderID: string(req.ClientOrderID),
		Venue:         string(req.Instrument.Venue),
		Base:          string(req.Instrument.Base),
		Quote:         string(req.Instrument.Quote),
		VenueSymbol:   req.Instrument.VenueSymbol,
		Side:          string(req.Side),
		Type:          string(req.Type),
		Price:         req.Price,
		Qty:           req.Qty,
		BotID:         req.BotID,
	})
	if err != nil {
		return fmt.Errorf("postgres: create pending order: %w", err)
	}
	return nil
}

// ApplyEvent locks the order row, decides via the domain state machine,
// and persists whatever the decision carries. Events that contribute
// nothing write nothing; the caller counts them from Decision.Drop.
func (s *OrderStore) ApplyEvent(ctx context.Context, source order.Source, ev order.Event) (order.Decision, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return order.Decision{}, fmt.Errorf("postgres: begin apply event: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := s.q.WithTx(tx)
	row, err := q.GetOrderForUpdate(ctx, string(ev.Ref.ClientOrderID))
	if errors.Is(err, pgx.ErrNoRows) {
		return order.Decision{}, ports.ErrNotFound
	}
	if err != nil {
		return order.Decision{}, fmt.Errorf("postgres: lock order: %w", err)
	}

	decision, err := order.Transition(order.State{Status: order.Status(row.Status), FilledQty: row.FilledQty}, ev)
	if err != nil {
		return order.Decision{}, err
	}
	if !decision.Transition && !decision.FillDelta.IsPositive() {
		return decision, nil
	}

	if err := s.persistDecision(ctx, q, row, source, ev, decision); err != nil {
		return order.Decision{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return order.Decision{}, fmt.Errorf("postgres: commit apply event: %w", err)
	}
	return decision, nil
}

func (s *OrderStore) persistDecision(ctx context.Context, q *sqlcgen.Queries, row sqlcgen.Order, source order.Source, ev order.Event, decision order.Decision) error {
	newFilled := row.FilledQty.Add(decision.FillDelta)
	tr, err := q.InsertTransition(ctx, sqlcgen.InsertTransitionParams{
		ClientOrderID: row.ClientOrderID,
		FromStatus:    row.Status,
		ToStatus:      string(decision.To),
		FilledQty:     newFilled,
		Source:        string(source),
		Reason:        nullString(ev.Reason),
		OccurredAt:    ev.At.UTC(),
	})
	if err != nil {
		return fmt.Errorf("postgres: insert transition: %w", err)
	}

	if decision.FillDelta.IsPositive() {
		// A conflict on venue_fill_id means this exact fill is already
		// recorded; the cumulative update below still applies venue truth.
		if _, err := q.InsertFill(ctx, sqlcgen.InsertFillParams{
			ClientOrderID: row.ClientOrderID,
			TransitionID:  tr.ID,
			Qty:           decision.FillDelta,
			Price:         nullNumeric(ev.FillPrice),
			Fee:           nullNumeric(ev.Fee),
			FeeCurrency:   nullString(string(ev.FeeCurrency)),
			VenueFillID:   nullString(ev.VenueFillID),
			OccurredAt:    ev.At.UTC(),
		}); err != nil {
			return fmt.Errorf("postgres: insert fill: %w", err)
		}
	}

	if err := q.ApplyOrderUpdate(ctx, sqlcgen.ApplyOrderUpdateParams{
		ClientOrderID: row.ClientOrderID,
		Status:        string(decision.To),
		FilledQty:     newFilled,
		AvgFillPrice:  avgFillPrice(row, decision, ev, newFilled),
		VenueOrderID:  nullString(ev.Ref.VenueOrderID),
		Reason:        nullString(ev.Reason),
	}); err != nil {
		return fmt.Errorf("postgres: update order: %w", err)
	}

	return s.insertEventOutbox(ctx, q, row, source, ev, decision, newFilled)
}

func (s *OrderStore) insertEventOutbox(ctx context.Context, q *sqlcgen.Queries, row sqlcgen.Order, source order.Source, ev order.Event, decision order.Decision, newFilled decimal.Decimal) error {
	id := order.ClientOrderID(row.ClientOrderID)
	venue := instrument.VenueID(row.Venue)
	if err := insertOutboxJSON(ctx, q, order.SubjectUpdated, order.UpdatedPayload{
		ClientOrderID: id,
		Venue:         venue,
		Status:        decision.To,
		FilledQty:     newFilled,
		Source:        source,
		At:            ev.At.UTC(),
	}); err != nil {
		return err
	}
	if !decision.FillDelta.IsPositive() {
		return nil
	}
	return insertOutboxJSON(ctx, q, order.SubjectFilled, order.FilledPayload{
		ClientOrderID: id,
		Venue:         venue,
		Qty:           decision.FillDelta,
		Price:         ev.FillPrice,
		Fee:           ev.Fee,
		FeeCurrency:   ev.FeeCurrency,
		VenueFillID:   ev.VenueFillID,
		At:            ev.At.UTC(),
	})
}

func insertOutboxJSON(ctx context.Context, q *sqlcgen.Queries, subject string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("postgres: marshal %s payload: %w", subject, err)
	}
	if err := q.InsertOutbox(ctx, sqlcgen.InsertOutboxParams{Subject: subject, Payload: body}); err != nil {
		return fmt.Errorf("postgres: insert outbox %s: %w", subject, err)
	}
	return nil
}

// avgFillPrice folds the new fill into the volume-weighted average. The
// stored average is kept when the event carries no usable price.
func avgFillPrice(row sqlcgen.Order, decision order.Decision, ev order.Event, newFilled decimal.Decimal) pgtype.Numeric {
	if !decision.FillDelta.IsPositive() || !ev.FillPrice.IsPositive() || newFilled.IsZero() {
		return row.AvgFillPrice
	}
	if !row.AvgFillPrice.Valid && row.FilledQty.IsPositive() {
		// Earlier fills carried no price (reconcile-sourced), so a true
		// weighted average is not computable. Start from the first known
		// price instead of weighting the unknown portion at zero.
		return nullNumeric(ev.FillPrice)
	}
	oldAvg := fromNumeric(row.AvgFillPrice)
	oldFilled := row.FilledQty
	newAvg := oldAvg.Mul(oldFilled).Add(ev.FillPrice.Mul(decision.FillDelta)).Div(newFilled)
	return nullNumeric(newAvg)
}

// MarkCancelRequested stamps the cancel intent; the first timestamp wins.
func (s *OrderStore) MarkCancelRequested(ctx context.Context, id order.ClientOrderID, at time.Time) error {
	n, err := s.q.MarkCancelRequested(ctx, sqlcgen.MarkCancelRequestedParams{
		ClientOrderID:     string(id),
		CancelRequestedAt: pgtype.Timestamptz{Time: at.UTC(), Valid: true},
	})
	if err != nil {
		return fmt.Errorf("postgres: mark cancel requested: %w", err)
	}
	if n == 0 {
		return ports.ErrNotFound
	}
	return nil
}

// GetOrder returns the stored order, or ports.ErrNotFound.
func (s *OrderStore) GetOrder(ctx context.Context, id order.ClientOrderID) (ports.StoredOrder, error) {
	row, err := s.q.GetOrder(ctx, string(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.StoredOrder{}, ports.ErrNotFound
	}
	if err != nil {
		return ports.StoredOrder{}, fmt.Errorf("postgres: get order: %w", err)
	}
	return storedOrder(row), nil
}

func storedOrder(row sqlcgen.Order) ports.StoredOrder {
	var cancelRequestedAt time.Time
	if row.CancelRequestedAt.Valid {
		cancelRequestedAt = row.CancelRequestedAt.Time
	}
	return ports.StoredOrder{
		ClientOrderID: order.ClientOrderID(row.ClientOrderID),
		BotID:         row.BotID,
		Instrument: instrument.Instrument{
			Venue: instrument.VenueID(row.Venue),
			// Only spot trades in M2; an instrument type column arrives
			// with derivatives.
			Type:        instrument.TypeSpot,
			Base:        money.Currency(row.Base),
			Quote:       money.Currency(row.Quote),
			VenueSymbol: row.VenueSymbol,
		},
		Side:              order.Side(row.Side),
		Type:              order.Type(row.Type),
		Price:             row.Price,
		Qty:               row.Qty,
		FilledQty:         row.FilledQty,
		AvgFillPrice:      fromNumeric(row.AvgFillPrice),
		Status:            order.Status(row.Status),
		VenueOrderID:      fromNullString(row.VenueOrderID),
		CancelRequestedAt: cancelRequestedAt,
		Reason:            fromNullString(row.Reason),
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
	}
}

func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func fromNullString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func nullNumeric(d decimal.Decimal) pgtype.Numeric {
	if d.IsZero() {
		return pgtype.Numeric{}
	}
	return pgtype.Numeric{Int: d.Coefficient(), Exp: d.Exponent(), Valid: true}
}

func fromNumeric(n pgtype.Numeric) decimal.Decimal {
	if !n.Valid || n.Int == nil {
		return decimal.Decimal{}
	}
	return decimal.NewFromBigInt(new(big.Int).Set(n.Int), n.Exp)
}
