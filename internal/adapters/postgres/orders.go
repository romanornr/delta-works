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
	"github.com/romanornr/delta-works/internal/domain/ledger"
	"github.com/romanornr/delta-works/internal/domain/money"
	"github.com/romanornr/delta-works/internal/domain/order"
	"github.com/romanornr/delta-works/internal/ports"
)

// OrderStore persists orders per the order state machine. ApplyEvent is the
// single write path for venue events: transition row, fill row and outbox
// rows commit atomically (docs/specs/manual-trading.md, ADR-0008).
type OrderStore struct {
	pool     *pgxpool.Pool
	q        *sqlcgen.Queries
	selector ledger.LotSelector
}

var (
	_ ports.OrderCommandStore   = (*OrderStore)(nil)
	_ ports.OrderEventStore     = (*OrderStore)(nil)
	_ ports.OrderReconcileStore = (*OrderStore)(nil)
	_ ports.OrderQueryStore     = (*OrderStore)(nil)
)

// NewOrderStore returns an OrderStore backed by pool.
func NewOrderStore(pool *pgxpool.Pool) *OrderStore {
	return &OrderStore{pool: pool, q: sqlcgen.New(pool), selector: ledger.FIFO{}}
}

// CreatePending inserts the pending row before the venue submit.
// Re-inserting the same ClientOrderID is a no-op, so submit retries with
// the same ULID are safe.
func (s *OrderStore) CreatePending(ctx context.Context, req order.Request) (bool, error) {
	n, err := s.q.InsertPendingOrder(ctx, sqlcgen.InsertPendingOrderParams{
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
		return false, fmt.Errorf("postgres: create pending order: %w", err)
	}
	return n == 1, nil
}

// ApplyEvent locks the order row, decides via the domain state machine,
// and persists whatever the decision carries. Dropped state events write
// nothing unless they supply a missing venue order ID.
func (s *OrderStore) ApplyEvent(ctx context.Context, source order.Source, ev order.Event) (order.ApplyResult, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return order.ApplyResult{}, fmt.Errorf("postgres: begin apply event: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := s.q.WithTx(tx)
	row, err := q.GetOrderForUpdate(ctx, string(ev.Ref.ClientOrderID))
	if errors.Is(err, pgx.ErrNoRows) {
		return order.ApplyResult{}, ports.ErrNotFound
	}
	if err != nil {
		return order.ApplyResult{}, fmt.Errorf("postgres: lock order: %w", err)
	}

	decision, err := order.Transition(order.State{Status: order.Status(row.Status), FilledQty: row.FilledQty}, ev)
	if err != nil {
		return order.ApplyResult{}, err
	}
	result := order.ApplyResult{Decision: decision}
	if !decision.Transition && !decision.FillDelta.IsPositive() {
		// A venue order ID is an identity fact even when the state event drops.
		adopted, err := adoptVenueOrderID(ctx, q, row, ev.Ref.VenueOrderID)
		if err != nil {
			return order.ApplyResult{}, err
		}
		if adopted {
			if err := tx.Commit(ctx); err != nil {
				return order.ApplyResult{}, fmt.Errorf("postgres: commit apply event: %w", err)
			}
		}
		return result, nil
	}

	if err := s.persistDecision(ctx, q, row, source, ev, &result); err != nil {
		return order.ApplyResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return order.ApplyResult{}, fmt.Errorf("postgres: commit apply event: %w", err)
	}
	return result, nil
}

func adoptVenueOrderID(ctx context.Context, q *sqlcgen.Queries, row sqlcgen.Order, venueOrderID string) (bool, error) {
	if row.VenueOrderID != nil || venueOrderID == "" {
		return false, nil
	}
	if _, err := q.AdoptVenueOrderID(ctx, sqlcgen.AdoptVenueOrderIDParams{
		ClientOrderID: row.ClientOrderID,
		VenueOrderID:  nullString(venueOrderID),
	}); err != nil {
		return false, fmt.Errorf("postgres: adopt venue order ID: %w", err)
	}
	return true, nil
}

func (s *OrderStore) persistDecision(ctx context.Context, q *sqlcgen.Queries, row sqlcgen.Order, source order.Source, ev order.Event, result *order.ApplyResult) error {
	decision := result.Decision
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
		fillConflict := false
		fill := sqlcgen.InsertFillParams{
			ClientOrderID: row.ClientOrderID,
			TransitionID:  tr.ID,
			Qty:           decision.FillDelta,
			Price:         nullNumeric(ev.FillPrice),
			Fee:           nullNumeric(ev.Fee),
			FeeCurrency:   nullString(string(ev.FeeCurrency)),
			VenueFillID:   nullString(ev.VenueFillID),
			OccurredAt:    ev.At.UTC(),
		}
		fillID, err := q.InsertFill(ctx, fill)
		if errors.Is(err, pgx.ErrNoRows) {
			fillConflict = true
			fill.VenueFillID = nil
			fill.Fee = pgtype.Numeric{}
			fill.FeeCurrency = nil
			fillID, err = q.InsertFill(ctx, fill)
			ev.VenueFillID = ""
			ev.Fee = decimal.Zero
			ev.FeeCurrency = ""
		}
		if err != nil {
			return fmt.Errorf("postgres: insert fill: %w", err)
		}
		result.Outcome, err = s.postLedgerFill(ctx, q, row, ev, decision.FillDelta, fillID)
		if err != nil {
			return err
		}
		result.FillConflict = fillConflict
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

	if err := s.insertEventOutbox(ctx, q, row, source, ev, decision, newFilled); err != nil {
		return err
	}
	return nil
}

func (s *OrderStore) insertEventOutbox(ctx context.Context, q *sqlcgen.Queries, row sqlcgen.Order, source order.Source, ev order.Event, decision order.Decision, newFilled decimal.Decimal) error {
	id := order.ClientOrderID(row.ClientOrderID)
	venue := instrument.VenueID(row.Venue)
	if err := insertOutboxJSON(ctx, q, order.SubjectUpdated, order.UpdatedPayload{
		ClientOrderID: id,
		Venue:         venue,
		Base:          money.Currency(row.Base),
		Quote:         money.Currency(row.Quote),
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
		Base:          money.Currency(row.Base),
		Quote:         money.Currency(row.Quote),
		Status:        decision.To,
		FilledQty:     newFilled,
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
func (s *OrderStore) GetOrder(ctx context.Context, id order.ClientOrderID) (order.Record, error) {
	row, err := s.q.GetOrder(ctx, string(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return order.Record{}, ports.ErrNotFound
	}
	if err != nil {
		return order.Record{}, fmt.Errorf("postgres: get order: %w", err)
	}
	return orderRecord(row), nil
}

// ListActiveOrders returns every non-terminal order for one venue.
func (s *OrderStore) ListActiveOrders(ctx context.Context, venue instrument.VenueID) ([]order.Record, error) {
	rows, err := s.q.ListActiveOrders(ctx, string(venue))
	if err != nil {
		return nil, fmt.Errorf("postgres: list active orders: %w", err)
	}
	orders := make([]order.Record, 0, len(rows))
	for _, row := range rows {
		orders = append(orders, orderRecord(row))
	}
	return orders, nil
}

// ListOrders returns a keyset-paginated order page.
func (s *OrderStore) ListOrders(ctx context.Context, query order.Query) ([]order.Record, error) {
	statuses := query.Statuses
	if len(statuses) == 0 {
		statuses = nil
	}
	var cursorCreatedAt pgtype.Timestamptz
	if query.CursorCreatedAt != nil {
		cursorCreatedAt = pgtype.Timestamptz{Time: query.CursorCreatedAt.UTC(), Valid: true}
	}
	rows, err := s.q.ListOrders(ctx, sqlcgen.ListOrdersParams{
		Venue: query.Venue, Statuses: statuses, BotID: query.BotID,
		CursorCreatedAt: cursorCreatedAt, CursorID: query.CursorID,
		RowLimit: int64(query.Limit) + 1,
	})
	if err != nil {
		return nil, fmt.Errorf("postgres: list orders: %w", err)
	}
	orders := make([]order.Record, 0, len(rows))
	for _, row := range rows {
		orders = append(orders, orderRecord(row))
	}
	return orders, nil
}

func orderRecord(row sqlcgen.Order) order.Record {
	var cancelRequestedAt time.Time
	if row.CancelRequestedAt.Valid {
		cancelRequestedAt = row.CancelRequestedAt.Time
	}
	return order.Record{
		ClientOrderID: order.ClientOrderID(row.ClientOrderID),
		BotID:         row.BotID,
		Instrument: instrument.Instrument{
			Venue: instrument.VenueID(row.Venue),
			// Only spot trades today; an instrument type column arrives
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
