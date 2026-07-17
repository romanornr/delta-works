package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"github.com/romanornr/delta-works/internal/adapters/postgres/sqlcgen"
	"github.com/romanornr/delta-works/internal/domain/instrument"
	"github.com/romanornr/delta-works/internal/domain/ledger"
	"github.com/romanornr/delta-works/internal/domain/money"
	"github.com/romanornr/delta-works/internal/domain/order"
	"github.com/romanornr/delta-works/internal/id"
)

const subjectUnmatchedSell = "ledger.unmatched_sell"

type unmatchedSellPayload struct {
	BotID         string              `json:"bot_id"`
	Venue         instrument.VenueID  `json:"venue"`
	Base          money.Currency      `json:"base"`
	Quote         money.Currency      `json:"quote"`
	ClientOrderID order.ClientOrderID `json:"client_order_id"`
	SellFillID    int64               `json:"sell_fill_id"`
	UnmatchedQty  string              `json:"unmatched_qty"`
}

// inventoryLockKey derives the advisory-lock key for one inventory tuple.
// Fields are length-prefixed before hashing so no separator value inside a
// field can make two different tuples collide.
func inventoryLockKey(fields ...string) int64 {
	h := sha256.New()
	for _, f := range fields {
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(len(f)))
		h.Write(n[:])
		h.Write([]byte(f))
	}
	// #nosec G115 -- any 64-bit pattern is a valid advisory lock key; the
	// signed reinterpretation is intended.
	return int64(binary.BigEndian.Uint64(h.Sum(nil)[:8]))
}

func (s *OrderStore) postLedgerFill(
	ctx context.Context,
	q *sqlcgen.Queries,
	row sqlcgen.Order,
	ev order.Event,
	fillQty decimal.Decimal,
	fillID int64,
) (ledger.Outcome, error) {
	if !fillQty.IsPositive() {
		return ledger.Outcome{}, fmt.Errorf("postgres: post ledger fill: quantity %s must be positive", fillQty)
	}
	if ev.FillPrice.IsNegative() {
		return ledger.Outcome{}, fmt.Errorf("postgres: post ledger fill: price %s must not be negative", ev.FillPrice)
	}
	if err := q.LockInventory(ctx, inventoryLockKey(row.BotID, row.Venue, row.Base, row.Quote)); err != nil {
		return ledger.Outcome{}, fmt.Errorf("postgres: lock inventory: %w", err)
	}

	switch order.Side(row.Side) {
	case order.Buy:
		return s.openLot(ctx, q, row, ev, fillQty, fillID)
	case order.Sell:
		return s.closeLots(ctx, q, row, ev, fillQty, fillID)
	default:
		return ledger.Outcome{}, fmt.Errorf("postgres: post ledger fill: unsupported side %q", row.Side)
	}
}

func (*OrderStore) openLot(
	ctx context.Context,
	q *sqlcgen.Queries,
	row sqlcgen.Order,
	ev order.Event,
	fillQty decimal.Decimal,
	fillID int64,
) (ledger.Outcome, error) {
	lotID := id.New()
	if err := q.InsertLot(ctx, sqlcgen.InsertLotParams{
		ID: lotID, BotID: row.BotID, Venue: row.Venue, Base: row.Base, Quote: row.Quote,
		Qty: fillQty, CostPrice: ev.FillPrice, OpenedByFillID: fillID, OpenedAt: ev.At.UTC(),
	}); err != nil {
		return ledger.Outcome{}, fmt.Errorf("postgres: insert lot: %w", err)
	}
	return ledger.Outcome{}, nil
}

func (s *OrderStore) closeLots(
	ctx context.Context,
	q *sqlcgen.Queries,
	row sqlcgen.Order,
	ev order.Event,
	fillQty decimal.Decimal,
	fillID int64,
) (ledger.Outcome, error) {
	rows, err := q.ListOpenLotsForUpdate(ctx, sqlcgen.ListOpenLotsForUpdateParams{
		BotID: row.BotID, Venue: row.Venue, Base: row.Base, Quote: row.Quote,
	})
	if err != nil {
		return ledger.Outcome{}, fmt.Errorf("postgres: list open lots: %w", err)
	}
	open := make([]ledger.Lot, 0, len(rows))
	for _, lot := range rows {
		open = append(open, ledger.Lot{
			ID: lot.ID, BotID: lot.BotID, Venue: instrument.VenueID(lot.Venue),
			Base: money.Currency(lot.Base), Quote: money.Currency(lot.Quote),
			Qty: lot.Qty, RemainingQty: lot.RemainingQty, CostPrice: lot.CostPrice, OpenedAt: lot.OpenedAt,
		})
	}
	allocation := s.selector.Select(open, fillQty)
	for _, closure := range allocation.Closures {
		if err := recordClosure(ctx, q, closure, ev.FillPrice, ev.At.UTC(), fillID); err != nil {
			return ledger.Outcome{}, err
		}
	}
	if !allocation.Unmatched.IsPositive() {
		return ledger.Outcome{}, nil
	}
	if err := recordUnmatched(ctx, q, row, ev, allocation.Unmatched, fillID); err != nil {
		return ledger.Outcome{}, err
	}
	return ledger.Outcome{UnmatchedQty: allocation.Unmatched}, nil
}

func recordClosure(
	ctx context.Context,
	q *sqlcgen.Queries,
	closure ledger.Closure,
	price decimal.Decimal,
	closedAt time.Time,
	fillID int64,
) error {
	if err := q.InsertLotClosure(ctx, sqlcgen.InsertLotClosureParams{
		LotID: closure.LotID, SellFillID: fillID, Qty: closure.Qty, Price: price, ClosedAt: closedAt,
	}); err != nil {
		return fmt.Errorf("postgres: insert lot closure: %w", err)
	}
	if err := q.DecrementLot(ctx, sqlcgen.DecrementLotParams{
		ID: closure.LotID, Qty: closure.Qty, ClosedAt: closedAt,
	}); err != nil {
		return fmt.Errorf("postgres: decrement lot: %w", err)
	}
	return nil
}

func recordUnmatched(
	ctx context.Context,
	q *sqlcgen.Queries,
	row sqlcgen.Order,
	ev order.Event,
	qty decimal.Decimal,
	fillID int64,
) error {
	if err := q.InsertUnmatchedSell(ctx, sqlcgen.InsertUnmatchedSellParams{
		SellFillID: fillID, BotID: row.BotID, Venue: row.Venue, Base: row.Base,
		Quote: row.Quote, Qty: qty, OccurredAt: ev.At.UTC(),
	}); err != nil {
		return fmt.Errorf("postgres: insert unmatched sell: %w", err)
	}
	return insertOutboxJSON(ctx, q, subjectUnmatchedSell, unmatchedSellPayload{
		BotID: row.BotID, Venue: instrument.VenueID(row.Venue),
		Base: money.Currency(row.Base), Quote: money.Currency(row.Quote),
		ClientOrderID: order.ClientOrderID(row.ClientOrderID), SellFillID: fillID,
		UnmatchedQty: qty.String(),
	})
}
