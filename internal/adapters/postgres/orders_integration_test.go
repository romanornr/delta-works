//go:build integration

package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/romanornr/delta-works/internal/domain/instrument"
	"github.com/romanornr/delta-works/internal/domain/money"
	"github.com/romanornr/delta-works/internal/domain/order"
	"github.com/romanornr/delta-works/internal/id"
	"github.com/romanornr/delta-works/internal/ports"
)

func testInstrument() instrument.Instrument {
	return instrument.Instrument{
		Venue:       "bybit",
		Type:        instrument.TypeSpot,
		Base:        money.Currency("BTC"),
		Quote:       money.Currency("USDT"),
		VenueSymbol: "BTCUSDT",
	}
}

func newPendingOrder(ctx context.Context, t *testing.T, store *OrderStore) order.Request {
	t.Helper()
	req := order.Request{
		ClientOrderID: order.ClientOrderID(id.New()),
		BotID:         "manual",
		Instrument:    testInstrument(),
		Side:          order.Buy,
		Type:          order.Limit,
		Price:         decimal.RequireFromString("50000"),
		Qty:           decimal.RequireFromString("1"),
	}
	if err := store.CreatePending(ctx, req); err != nil {
		t.Fatalf("CreatePending: %v", err)
	}
	return req
}

func fillEvent(req order.Request, status order.Status, cumulative, price string) order.Event {
	return order.Event{
		Ref:       order.Ref{Instrument: req.Instrument, ClientOrderID: req.ClientOrderID, VenueOrderID: "v-" + string(req.ClientOrderID)},
		Status:    status,
		FilledQty: decimal.RequireFromString(cumulative),
		FillPrice: decimal.RequireFromString(price),
		At:        time.Now().UTC(),
	}
}

func countRows(ctx context.Context, t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(ctx, query, args...).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", query, err)
	}
	return n
}

func TestOrderStoreApplyEvent(t *testing.T) {
	ctx := context.Background()
	pool, err := Connect(ctx, startPostgres(t))
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer pool.Close()
	store := NewOrderStore(pool)

	t.Run("idempotent apply", func(t *testing.T) {
		req := newPendingOrder(ctx, t, store)
		if err := store.CreatePending(ctx, req); err != nil {
			t.Fatalf("CreatePending retry: %v", err)
		}

		ev := fillEvent(req, order.StatusOpen, "0", "0")
		d, _, err := store.ApplyEvent(ctx, order.SourceAck, ev)
		if err != nil || !d.Transition {
			t.Fatalf("first apply: decision=%+v err=%v", d, err)
		}
		d, _, err = store.ApplyEvent(ctx, order.SourceStream, ev)
		if err != nil || d.Transition || d.Drop != order.DropDuplicate {
			t.Fatalf("replay: decision=%+v err=%v", d, err)
		}

		transitions := countRows(ctx, t, pool, "SELECT COUNT(*) FROM order_transitions WHERE client_order_id=$1", string(req.ClientOrderID))
		if transitions != 1 {
			t.Fatalf("transitions = %d, want 1", transitions)
		}
		got, err := store.GetOrder(ctx, req.ClientOrderID)
		if err != nil || got.Status != order.StatusOpen || got.VenueOrderID != "v-"+string(req.ClientOrderID) {
			t.Fatalf("GetOrder = %+v, err=%v", got, err)
		}
	})

	t.Run("dropped event adopts venue order ID once", func(t *testing.T) {
		req := newPendingOrder(ctx, t, store)
		ev := fillEvent(req, order.StatusPending, "0", "0")
		ev.Ref.VenueOrderID = "v-1"
		decision, _, err := store.ApplyEvent(ctx, order.SourceReconcile, ev)
		if err != nil || decision.Drop != order.DropDuplicate || decision.Transition {
			t.Fatalf("first apply: decision=%+v err=%v", decision, err)
		}
		got, err := store.GetOrder(ctx, req.ClientOrderID)
		if err != nil || got.VenueOrderID != "v-1" {
			t.Fatalf("GetOrder = %+v, err=%v; want venue order ID v-1", got, err)
		}

		ev.Ref.VenueOrderID = "v-2"
		decision, _, err = store.ApplyEvent(ctx, order.SourceReconcile, ev)
		if err != nil || decision.Drop != order.DropDuplicate || decision.Transition {
			t.Fatalf("second apply: decision=%+v err=%v", decision, err)
		}
		got, err = store.GetOrder(ctx, req.ClientOrderID)
		if err != nil || got.VenueOrderID != "v-1" {
			t.Fatalf("GetOrder = %+v, err=%v; venue order ID must stay v-1", got, err)
		}
	})

	t.Run("out of order convergence", func(t *testing.T) {
		req := newPendingOrder(ctx, t, store)
		if _, _, err := store.ApplyEvent(ctx, order.SourceStream, fillEvent(req, order.StatusFilled, "1", "50000")); err != nil {
			t.Fatalf("filled event: %v", err)
		}
		d, _, err := store.ApplyEvent(ctx, order.SourceStream, fillEvent(req, order.StatusPartiallyFilled, "0.4", "50000"))
		if err != nil || d.Drop != order.DropTerminal || !d.FillDelta.IsZero() {
			t.Fatalf("stale partial after filled: decision=%+v err=%v", d, err)
		}

		got, err := store.GetOrder(ctx, req.ClientOrderID)
		if err != nil || got.Status != order.StatusFilled || !got.FilledQty.Equal(decimal.RequireFromString("1")) {
			t.Fatalf("GetOrder = %+v, err=%v", got, err)
		}
		fills := countRows(ctx, t, pool, "SELECT COUNT(*) FROM fills WHERE client_order_id=$1", string(req.ClientOrderID))
		if fills != 1 {
			t.Fatalf("fills = %d, want 1", fills)
		}
	})

	t.Run("fill deltas and venue fill dedupe", func(t *testing.T) {
		req := newPendingOrder(ctx, t, store)
		partial := fillEvent(req, order.StatusPartiallyFilled, "0.4", "50000")
		partial.VenueFillID = "f-1"
		if _, _, err := store.ApplyEvent(ctx, order.SourceStream, partial); err != nil {
			t.Fatalf("partial: %v", err)
		}
		if _, _, err := store.ApplyEvent(ctx, order.SourceStream, partial); err != nil {
			t.Fatalf("partial replay: %v", err)
		}
		final := fillEvent(req, order.StatusFilled, "1", "51000")
		final.VenueFillID = "f-2"
		if _, _, err := store.ApplyEvent(ctx, order.SourceStream, final); err != nil {
			t.Fatalf("final: %v", err)
		}

		fills := countRows(ctx, t, pool, "SELECT COUNT(*) FROM fills WHERE client_order_id=$1", string(req.ClientOrderID))
		if fills != 2 {
			t.Fatalf("fills = %d, want 2", fills)
		}
		got, err := store.GetOrder(ctx, req.ClientOrderID)
		if err != nil {
			t.Fatalf("GetOrder: %v", err)
		}
		// 0.4 at 50000 plus 0.6 at 51000 is a weighted average of 50600.
		if !got.AvgFillPrice.Equal(decimal.RequireFromString("50600")) {
			t.Fatalf("AvgFillPrice = %s, want 50600", got.AvgFillPrice)
		}
	})

	t.Run("unpriced fill does not skew average", func(t *testing.T) {
		req := newPendingOrder(ctx, t, store)
		unpriced := fillEvent(req, order.StatusPartiallyFilled, "0.5", "0")
		if _, _, err := store.ApplyEvent(ctx, order.SourceReconcile, unpriced); err != nil {
			t.Fatalf("unpriced: %v", err)
		}
		priced := fillEvent(req, order.StatusFilled, "1", "50000")
		if _, _, err := store.ApplyEvent(ctx, order.SourceStream, priced); err != nil {
			t.Fatalf("priced: %v", err)
		}

		got, err := store.GetOrder(ctx, req.ClientOrderID)
		if err != nil {
			t.Fatalf("GetOrder: %v", err)
		}
		// The unpriced half must not be weighted at zero; the average
		// starts from the first known price.
		if !got.AvgFillPrice.Equal(decimal.RequireFromString("50000")) {
			t.Fatalf("AvgFillPrice = %s, want 50000", got.AvgFillPrice)
		}
	})

	t.Run("cancel intent keeps first timestamp", func(t *testing.T) {
		req := newPendingOrder(ctx, t, store)
		first := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
		if err := store.MarkCancelRequested(ctx, req.ClientOrderID, first); err != nil {
			t.Fatalf("MarkCancelRequested: %v", err)
		}
		if err := store.MarkCancelRequested(ctx, req.ClientOrderID, first.Add(time.Hour)); err != nil {
			t.Fatalf("MarkCancelRequested repeat: %v", err)
		}
		got, err := store.GetOrder(ctx, req.ClientOrderID)
		if err != nil || !got.CancelRequestedAt.Equal(first) {
			t.Fatalf("CancelRequestedAt = %v, err=%v; want %v", got.CancelRequestedAt, err, first)
		}
		if err := store.MarkCancelRequested(ctx, "absent", first); !errors.Is(err, ports.ErrNotFound) {
			t.Fatalf("unknown order err = %v, want ErrNotFound", err)
		}
	})

	t.Run("unknown order", func(t *testing.T) {
		ev := fillEvent(order.Request{ClientOrderID: order.ClientOrderID(id.New()), Instrument: testInstrument()}, order.StatusOpen, "0", "0")
		if _, _, err := store.ApplyEvent(ctx, order.SourceStream, ev); !errors.Is(err, ports.ErrNotFound) {
			t.Fatalf("ApplyEvent unknown order: err=%v, want ErrNotFound", err)
		}
	})
}

func TestOrderStoreListActiveOrders(t *testing.T) {
	ctx := context.Background()
	pool, err := Connect(ctx, startPostgres(t))
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer pool.Close()
	store := NewOrderStore(pool)

	create := func(venue instrument.VenueID) order.Request {
		t.Helper()
		req := order.Request{
			ClientOrderID: order.ClientOrderID(id.New()), BotID: "manual",
			Instrument: testInstrument(), Side: order.Buy, Type: order.Limit,
			Price: decimal.RequireFromString("50000"), Qty: decimal.RequireFromString("1"),
		}
		req.Instrument.Venue = venue
		if err := store.CreatePending(ctx, req); err != nil {
			t.Fatalf("CreatePending: %v", err)
		}
		return req
	}

	pending := create("bybit")
	open := create("bybit")
	partial := create("bybit")
	filled := create("bybit")
	otherVenue := create("kraken")
	for _, event := range []order.Event{
		fillEvent(open, order.StatusOpen, "0", "0"),
		fillEvent(partial, order.StatusPartiallyFilled, "0.4", "50000"),
		fillEvent(filled, order.StatusFilled, "1", "50000"),
		fillEvent(otherVenue, order.StatusOpen, "0", "0"),
	} {
		if _, _, err := store.ApplyEvent(ctx, order.SourceStream, event); err != nil {
			t.Fatalf("ApplyEvent: %v", err)
		}
	}

	got, err := store.ListActiveOrders(ctx, "bybit")
	if err != nil {
		t.Fatalf("ListActiveOrders: %v", err)
	}
	want := map[order.ClientOrderID]order.Status{
		pending.ClientOrderID: order.StatusPending,
		open.ClientOrderID:    order.StatusOpen,
		partial.ClientOrderID: order.StatusPartiallyFilled,
	}
	if len(got) != len(want) {
		t.Fatalf("ListActiveOrders returned %d orders, want %d: %+v", len(got), len(want), got)
	}
	for _, stored := range got {
		if stored.Instrument.Venue != "bybit" || want[stored.ClientOrderID] != stored.Status {
			t.Fatalf("unexpected active order: %+v", stored)
		}
		delete(want, stored.ClientOrderID)
	}
	if len(want) != 0 {
		t.Fatalf("missing active orders: %v", want)
	}
}

func TestOutboxRoundTrip(t *testing.T) {
	ctx := context.Background()
	pool, err := Connect(ctx, startPostgres(t))
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer pool.Close()
	orders := NewOrderStore(pool)
	outbox := NewOutboxStore(pool)

	req := newPendingOrder(ctx, t, orders)
	if _, _, err := orders.ApplyEvent(ctx, order.SourceAck, fillEvent(req, order.StatusOpen, "0", "0")); err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, _, err := orders.ApplyEvent(ctx, order.SourceStream, fillEvent(req, order.StatusFilled, "1", "50000")); err != nil {
		t.Fatalf("filled: %v", err)
	}

	rows, oldest, err := outbox.UnpublishedStats(ctx)
	if err != nil || rows != 3 || oldest.IsZero() {
		t.Fatalf("UnpublishedStats = %d rows, oldest %v, err=%v; want 3 rows", rows, oldest, err)
	}

	var subjects []string
	published, err := outbox.PublishPending(ctx, 100, func(m ports.OutboxMessage) error {
		subjects = append(subjects, m.Subject)
		return nil
	})
	if err != nil || published != 3 {
		t.Fatalf("PublishPending = %d, err=%v; want 3", published, err)
	}
	want := []string{order.SubjectUpdated, order.SubjectUpdated, order.SubjectFilled}
	for i, s := range want {
		if subjects[i] != s {
			t.Fatalf("subjects = %v, want %v", subjects, want)
		}
	}

	published, err = outbox.PublishPending(ctx, 100, func(ports.OutboxMessage) error { return nil })
	if err != nil || published != 0 {
		t.Fatalf("second PublishPending = %d, err=%v; want 0", published, err)
	}

	deleted, err := outbox.DeletePublishedBefore(ctx, time.Now().UTC().Add(time.Minute))
	if err != nil || deleted != 3 {
		t.Fatalf("DeletePublishedBefore = %d, err=%v; want 3", deleted, err)
	}
}

func TestOutboxPublishErrorKeepsRows(t *testing.T) {
	ctx := context.Background()
	pool, err := Connect(ctx, startPostgres(t))
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer pool.Close()
	orders := NewOrderStore(pool)
	outbox := NewOutboxStore(pool)

	req := newPendingOrder(ctx, t, orders)
	if _, _, err := orders.ApplyEvent(ctx, order.SourceAck, fillEvent(req, order.StatusOpen, "0", "0")); err != nil {
		t.Fatalf("open: %v", err)
	}

	if _, err := outbox.PublishPending(ctx, 100, func(ports.OutboxMessage) error {
		return context.Canceled
	}); err == nil {
		t.Fatal("PublishPending with failing publish: want error")
	}

	rows, _, err := outbox.UnpublishedStats(ctx)
	if err != nil || rows != 1 {
		t.Fatalf("UnpublishedStats after failed publish = %d, err=%v; want 1", rows, err)
	}
}
