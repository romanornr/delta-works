//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/romanornr/delta-works/internal/domain/order"
	"github.com/romanornr/delta-works/internal/events"
	"github.com/romanornr/delta-works/internal/id"
)

func newLedgerOrder(
	ctx context.Context,
	t *testing.T,
	store *OrderStore,
	bot string,
	side order.Side,
	qty string,
) order.Request {
	t.Helper()
	req := order.Request{
		ClientOrderID: order.ClientOrderID(id.New()), BotID: bot,
		Instrument: testInstrument(), Side: side, Type: order.Limit,
		Price: decimal.RequireFromString("50000"), Qty: decimal.RequireFromString(qty),
	}
	if _, err := store.CreatePending(ctx, req); err != nil {
		t.Fatalf("CreatePending: %v", err)
	}
	return req
}

func ledgerEvent(
	req order.Request,
	status order.Status,
	cumulative, price, venueFillID string,
	at time.Time,
) order.Event {
	event := fillEvent(req, status, cumulative, price)
	event.VenueFillID = venueFillID
	event.At = at
	return event
}

func applyLedgerEvent(
	ctx context.Context,
	t *testing.T,
	store *OrderStore,
	source order.Source,
	event order.Event,
) order.ApplyResult {
	t.Helper()
	result, err := store.ApplyEvent(ctx, source, event)
	if err != nil {
		t.Fatalf("ApplyEvent: %v", err)
	}
	return result
}

func TestOrderStoreLedgerPosting(t *testing.T) {
	ctx := context.Background()
	pool, err := Connect(ctx, startPostgres(t))
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer pool.Close()
	store := NewOrderStore(pool)

	t.Run("buy opens lot and venue fill replay is idempotent", func(t *testing.T) {
		req := newLedgerOrder(ctx, t, store, "buy-replay", order.Buy, "1")
		at := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
		event := ledgerEvent(req, order.StatusFilled, "1", "50000", "buy-replay-fill", at)
		applyLedgerEvent(ctx, t, store, order.SourceStream, event)

		var qty, remaining, cost decimal.Decimal
		var openedAt time.Time
		if err := pool.QueryRow(ctx, `
	SELECT l.qty, l.remaining_qty, l.cost_price, l.opened_at
	FROM lots l JOIN fills f ON f.id=l.opened_by_fill_id
	WHERE f.client_order_id=$1`, string(req.ClientOrderID)).
			Scan(&qty, &remaining, &cost, &openedAt); err != nil {
			t.Fatalf("query lot: %v", err)
		}
		if !qty.Equal(decimal.NewFromInt(1)) || !remaining.Equal(qty) ||
			!cost.Equal(decimal.NewFromInt(50000)) || !openedAt.Equal(at) {
			t.Fatalf("lot qty=%s remaining=%s cost=%s opened=%v", qty, remaining, cost, openedAt)
		}
		outboxBefore := countRows(ctx, t, pool, "SELECT COUNT(*) FROM outbox")
		applyLedgerEvent(ctx, t, store, order.SourceStream, event)
		if got := countRows(ctx, t, pool, "SELECT COUNT(*) FROM lots WHERE bot_id=$1", req.BotID); got != 1 {
			t.Fatalf("lots = %d, want 1", got)
		}
		if got := countRows(ctx, t, pool, "SELECT COUNT(*) FROM lot_closures"); got != 0 {
			t.Fatalf("closures = %d, want 0", got)
		}
		if got := countRows(ctx, t, pool, "SELECT COUNT(*) FROM unmatched_sells"); got != 0 {
			t.Fatalf("unmatched sells = %d, want 0", got)
		}
		if got := countRows(ctx, t, pool, "SELECT COUNT(*) FROM outbox"); got != outboxBefore {
			t.Fatalf("outbox rows after replay = %d, want %d", got, outboxBefore)
		}
	})

	t.Run("reused venue fill ID posts every cumulative delta", func(t *testing.T) {
		req := newLedgerOrder(ctx, t, store, "fill-conflict", order.Buy, "1")
		at := time.Date(2026, 7, 12, 10, 30, 0, 0, time.UTC)
		first := ledgerEvent(req, order.StatusPartiallyFilled, "0.4", "100", "reused-fill", at)
		first.Fee = decimal.RequireFromString("0.01")
		first.FeeCurrency = "USDT"
		firstResult := applyLedgerEvent(ctx, t, store, order.SourceStream, first)
		assertBuyOrderAccounting(ctx, t, pool, req.ClientOrderID, "0.4")

		second := ledgerEvent(req, order.StatusFilled, "1", "110", "reused-fill", at.Add(time.Minute))
		second.Fee = decimal.RequireFromString("0.02")
		second.FeeCurrency = "USDT"
		secondResult := applyLedgerEvent(ctx, t, store, order.SourceStream, second)
		assertBuyOrderAccounting(ctx, t, pool, req.ClientOrderID, "1")

		if firstResult.FillConflict || !secondResult.FillConflict {
			t.Fatalf("fill conflicts: first=%t second=%t, want false/true", firstResult.FillConflict, secondResult.FillConflict)
		}

		rows, err := pool.Query(ctx, `
SELECT l.qty, l.cost_price
FROM lots l
JOIN fills f ON f.id=l.opened_by_fill_id
WHERE f.client_order_id=$1
ORDER BY l.opened_at, l.id`, string(req.ClientOrderID))
		if err != nil {
			t.Fatalf("query conflict lots: %v", err)
		}
		defer rows.Close()
		wantQty := []decimal.Decimal{decimal.RequireFromString("0.4"), decimal.RequireFromString("0.6")}
		wantCost := []decimal.Decimal{decimal.NewFromInt(100), decimal.NewFromInt(110)}
		i := 0
		for rows.Next() {
			var qty, cost decimal.Decimal
			if err := rows.Scan(&qty, &cost); err != nil {
				t.Fatalf("scan conflict lot: %v", err)
			}
			if i >= len(wantQty) || !qty.Equal(wantQty[i]) || !cost.Equal(wantCost[i]) {
				t.Fatalf("lot %d: qty=%s cost=%s", i, qty, cost)
			}
			i++
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("conflict lots: %v", err)
		}
		if i != len(wantQty) {
			t.Fatalf("conflict lots = %d, want %d", i, len(wantQty))
		}

		var originalQty, originalFee decimal.Decimal
		var originalFeeCurrency string
		if err := pool.QueryRow(ctx, `
SELECT qty, fee, fee_currency FROM fills
WHERE client_order_id=$1 AND venue_fill_id='reused-fill'`, string(req.ClientOrderID)).
			Scan(&originalQty, &originalFee, &originalFeeCurrency); err != nil {
			t.Fatalf("query identified fill: %v", err)
		}
		if !originalQty.Equal(decimal.RequireFromString("0.4")) ||
			!originalFee.Equal(decimal.RequireFromString("0.01")) || originalFeeCurrency != "USDT" {
			t.Fatalf("identified fill: qty=%s fee=%s %s", originalQty, originalFee, originalFeeCurrency)
		}
		if got := countRows(ctx, t, pool, `
SELECT COUNT(*) FROM fills
WHERE client_order_id=$1 AND venue_fill_id IS NULL AND fee IS NULL AND fee_currency IS NULL`, string(req.ClientOrderID)); got != 1 {
			t.Fatalf("fallback fills without conflicting metadata = %d, want 1", got)
		}

		var body []byte
		if err := pool.QueryRow(ctx, `
SELECT payload FROM outbox
WHERE subject='order.filled' AND payload->>'client_order_id'=$1
ORDER BY id DESC LIMIT 1`, string(req.ClientOrderID)).Scan(&body); err != nil {
			t.Fatalf("query fallback outbox: %v", err)
		}
		var payload events.OrderFilledPayload
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("decode fallback outbox: %v", err)
		}
		if payload.VenueFillID != "" || !payload.Fee.IsZero() || payload.FeeCurrency != "" {
			t.Fatalf("fallback outbox retained conflicting metadata: %+v", payload)
		}
	})

	t.Run("sell closes FIFO across lots", func(t *testing.T) {
		bot := "fifo"
		first := newLedgerOrder(ctx, t, store, bot, order.Buy, "2")
		second := newLedgerOrder(ctx, t, store, bot, order.Buy, "3")
		firstAt := time.Date(2026, 7, 12, 11, 0, 0, 0, time.UTC)
		secondAt := firstAt.Add(time.Minute)
		applyLedgerEvent(ctx, t, store, order.SourceStream,
			ledgerEvent(first, order.StatusFilled, "2", "100", "fifo-buy-1", firstAt))
		applyLedgerEvent(ctx, t, store, order.SourceStream,
			ledgerEvent(second, order.StatusFilled, "3", "110", "fifo-buy-2", secondAt))

		sell := newLedgerOrder(ctx, t, store, bot, order.Sell, "4")
		sellAt := secondAt.Add(time.Minute)
		result := applyLedgerEvent(ctx, t, store, order.SourceStream,
			ledgerEvent(sell, order.StatusFilled, "4", "120", "fifo-sell", sellAt))
		if result.UnmatchedQty.IsPositive() {
			t.Fatalf("sell outcome = %+v", result.Outcome)
		}

		rows, err := pool.Query(ctx, `
SELECT opening.client_order_id, c.qty, c.price, c.closed_at
FROM lot_closures c
JOIN lots l ON l.id=c.lot_id
JOIN fills opening ON opening.id=l.opened_by_fill_id
WHERE c.sell_fill_id = (SELECT id FROM fills WHERE client_order_id=$1)
ORDER BY c.id`, string(sell.ClientOrderID))
		if err != nil {
			t.Fatalf("query closures: %v", err)
		}
		defer rows.Close()
		wantIDs := []string{string(first.ClientOrderID), string(second.ClientOrderID)}
		wantQty := []decimal.Decimal{decimal.NewFromInt(2), decimal.NewFromInt(2)}
		i := 0
		for rows.Next() {
			var lotID string
			var qty, price decimal.Decimal
			var closedAt time.Time
			if err := rows.Scan(&lotID, &qty, &price, &closedAt); err != nil {
				t.Fatalf("scan closure: %v", err)
			}
			if i >= len(wantIDs) || lotID != wantIDs[i] || !qty.Equal(wantQty[i]) ||
				!price.Equal(decimal.NewFromInt(120)) || !closedAt.Equal(sellAt) {
				t.Fatalf("closure %d: lot=%s qty=%s price=%s at=%v", i, lotID, qty, price, closedAt)
			}
			i++
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("closures rows: %v", err)
		}
		if i != 2 {
			t.Fatalf("closures = %d, want 2", i)
		}
		assertLotState(ctx, t, pool, first.ClientOrderID, "0", "closed", sellAt)
		assertLotState(ctx, t, pool, second.ClientOrderID, "1", "open", time.Time{})
	})

	for _, tt := range []struct {
		name, bot, buyQty, sellQty, wantUnmatched string
	}{
		{name: "partial oversell", bot: "partial-oversell", buyQty: "1", sellQty: "2", wantUnmatched: "1"},
		{name: "full oversell", bot: "full-oversell", sellQty: "2", wantUnmatched: "2"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			at := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
			if tt.buyQty != "" {
				buy := newLedgerOrder(ctx, t, store, tt.bot, order.Buy, tt.buyQty)
				applyLedgerEvent(ctx, t, store, order.SourceStream,
					ledgerEvent(buy, order.StatusFilled, tt.buyQty, "100", tt.bot+"-buy", at))
			}
			sell := newLedgerOrder(ctx, t, store, tt.bot, order.Sell, tt.sellQty)
			event := ledgerEvent(sell, order.StatusFilled, tt.sellQty, "120", tt.bot+"-sell", at.Add(time.Minute))
			result := applyLedgerEvent(ctx, t, store, order.SourceStream, event)
			want := decimal.RequireFromString(tt.wantUnmatched)
			if !result.UnmatchedQty.Equal(want) {
				t.Fatalf("ledger outcome = %+v, want unmatched %s", result.Outcome, want)
			}
			assertUnmatchedEvent(ctx, t, pool, sell, want)
			replay := applyLedgerEvent(ctx, t, store, order.SourceStream, event)
			if replay.UnmatchedQty.IsPositive() {
				t.Fatalf("replay ledger outcome = %+v", replay.Outcome)
			}
			assertUnmatchedEvent(ctx, t, pool, sell, want)
		})
	}

	t.Run("reconcile cumulative fills post once per delta", func(t *testing.T) {
		req := newLedgerOrder(ctx, t, store, "reconcile", order.Buy, "2")
		at := time.Date(2026, 7, 12, 13, 0, 0, 0, time.UTC)
		first := ledgerEvent(req, order.StatusPartiallyFilled, "1", "100", "", at)
		applyLedgerEvent(ctx, t, store, order.SourceReconcile, first)
		applyLedgerEvent(ctx, t, store, order.SourceReconcile, first)
		second := ledgerEvent(req, order.StatusPartiallyFilled, "2", "110", "", at.Add(time.Minute))
		applyLedgerEvent(ctx, t, store, order.SourceReconcile, second)
		if got := countRows(ctx, t, pool, "SELECT COUNT(*) FROM fills WHERE client_order_id=$1", string(req.ClientOrderID)); got != 2 {
			t.Fatalf("fills = %d, want 2", got)
		}
		if got := countRows(ctx, t, pool, "SELECT COUNT(*) FROM lots WHERE bot_id=$1", req.BotID); got != 2 {
			t.Fatalf("lots = %d, want 2", got)
		}
	})

	t.Run("canceled event posts higher cumulative fill", func(t *testing.T) {
		req := newLedgerOrder(ctx, t, store, "cancel-fill", order.Buy, "2")
		at := time.Date(2026, 7, 12, 14, 0, 0, 0, time.UTC)
		applyLedgerEvent(ctx, t, store, order.SourceStream,
			ledgerEvent(req, order.StatusPartiallyFilled, "1", "100", "cancel-fill-1", at))
		applyLedgerEvent(ctx, t, store, order.SourceStream,
			ledgerEvent(req, order.StatusCanceled, "2", "110", "cancel-fill-2", at.Add(time.Minute)))
		stored, err := store.GetOrder(ctx, req.ClientOrderID)
		if err != nil || stored.Status != order.StatusCanceled || !stored.FilledQty.Equal(decimal.NewFromInt(2)) {
			t.Fatalf("GetOrder = %+v, err=%v", stored, err)
		}
		if got := countRows(ctx, t, pool, `
SELECT COUNT(*) FROM lots l
JOIN fills f ON f.id=l.opened_by_fill_id
JOIN order_transitions tr ON tr.id=f.transition_id
WHERE tr.client_order_id=$1 AND tr.to_status='canceled'`, string(req.ClientOrderID)); got != 1 {
			t.Fatalf("lots posted by canceled transition = %d, want 1", got)
		}
	})

	t.Run("zero price buy opens zero cost lot", func(t *testing.T) {
		req := newLedgerOrder(ctx, t, store, "zero-price", order.Buy, "1")
		applyLedgerEvent(ctx, t, store, order.SourceReconcile,
			ledgerEvent(req, order.StatusFilled, "1", "0", "", time.Date(2026, 7, 12, 15, 0, 0, 0, time.UTC)))
		var cost decimal.Decimal
		if err := pool.QueryRow(ctx, `
	SELECT l.cost_price FROM lots l JOIN fills f ON f.id=l.opened_by_fill_id
	WHERE f.client_order_id=$1`, string(req.ClientOrderID)).Scan(&cost); err != nil {
			t.Fatalf("query cost price: %v", err)
		}
		if !cost.IsZero() {
			t.Fatalf("cost price = %s, want zero", cost)
		}
	})

	assertLedgerCrossTableInvariant(ctx, t, pool)
}

func TestOrderStoreLedgerConcurrency(t *testing.T) {
	ctx := context.Background()
	pool, err := Connect(ctx, startPostgres(t))
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer pool.Close()
	store := NewOrderStore(pool)

	for i := range 20 {
		t.Run("buy-sell-"+id.New(), func(t *testing.T) {
			bot := "parallel-buy-sell-" + id.New()
			buy := newLedgerOrder(ctx, t, store, bot, order.Buy, "1")
			sell := newLedgerOrder(ctx, t, store, bot, order.Sell, "2")
			at := time.Date(2026, 7, 12, 16, 0, i, 0, time.UTC)
			buyEvent := ledgerEvent(buy, order.StatusFilled, "1", "100", bot+"-buy", at)
			sellEvent := ledgerEvent(sell, order.StatusFilled, "2", "110", bot+"-sell", at)
			_, sellResult := applyConcurrently(ctx, t, store, buyEvent, sellEvent)
			var unmatched decimal.Decimal
			if err := pool.QueryRow(ctx, `
SELECT qty FROM unmatched_sells
WHERE sell_fill_id=(SELECT id FROM fills WHERE client_order_id=$1)`, string(sell.ClientOrderID)).Scan(&unmatched); err != nil {
				t.Fatalf("query unmatched: %v", err)
			}
			var closed decimal.Decimal
			if err := pool.QueryRow(ctx, `
SELECT COALESCE(SUM(qty), 0) FROM lot_closures
WHERE sell_fill_id=(SELECT id FROM fills WHERE client_order_id=$1)`, string(sell.ClientOrderID)).Scan(&closed); err != nil {
				t.Fatalf("query closures: %v", err)
			}
			var remaining decimal.Decimal
			if err := pool.QueryRow(ctx, `
SELECT l.remaining_qty FROM lots l JOIN fills f ON f.id=l.opened_by_fill_id
WHERE f.client_order_id=$1`, string(buy.ClientOrderID)).Scan(&remaining); err != nil {
				t.Fatalf("query lot: %v", err)
			}
			if !closed.Add(unmatched).Equal(decimal.NewFromInt(2)) ||
				!closed.Add(remaining).Equal(decimal.NewFromInt(1)) || !sellResult.UnmatchedQty.Equal(unmatched) {
				t.Fatalf("closed=%s unmatched=%s remaining=%s outcome=%+v", closed, unmatched, remaining, sellResult.Outcome)
			}
		})
	}

	for range 20 {
		t.Run("two-sells-"+id.New(), func(t *testing.T) {
			bot := "parallel-sells-" + id.New()
			buy := newLedgerOrder(ctx, t, store, bot, order.Buy, "1")
			at := time.Date(2026, 7, 12, 17, 0, 0, 0, time.UTC)
			applyLedgerEvent(ctx, t, store, order.SourceStream,
				ledgerEvent(buy, order.StatusFilled, "1", "100", bot+"-buy", at))
			first := newLedgerOrder(ctx, t, store, bot, order.Sell, "1")
			second := newLedgerOrder(ctx, t, store, bot, order.Sell, "1")
			firstEvent := ledgerEvent(first, order.StatusFilled, "1", "110", bot+"-sell-1", at.Add(time.Minute))
			secondEvent := ledgerEvent(second, order.StatusFilled, "1", "110", bot+"-sell-2", at.Add(time.Minute))
			_, _ = applyConcurrently(ctx, t, store, firstEvent, secondEvent)
			var closed decimal.Decimal
			if err := pool.QueryRow(ctx, `
SELECT COALESCE(SUM(c.qty), 0)
FROM lot_closures c JOIN lots l ON l.id=c.lot_id
JOIN fills f ON f.id=l.opened_by_fill_id
WHERE f.client_order_id=$1`, string(buy.ClientOrderID)).Scan(&closed); err != nil {
				t.Fatalf("query closures: %v", err)
			}
			if closed.GreaterThan(decimal.NewFromInt(1)) || !closed.Equal(decimal.NewFromInt(1)) {
				t.Fatalf("lot closed quantity = %s, want 1", closed)
			}
			if got := countRows(ctx, t, pool, "SELECT COUNT(*) FROM unmatched_sells WHERE bot_id=$1", bot); got != 1 {
				t.Fatalf("unmatched sells = %d, want 1", got)
			}
		})
	}

	assertLedgerCrossTableInvariant(ctx, t, pool)
}

func applyConcurrently(
	ctx context.Context,
	t *testing.T,
	store *OrderStore,
	first, second order.Event,
) (order.ApplyResult, order.ApplyResult) {
	t.Helper()
	start := make(chan struct{})
	var results [2]order.ApplyResult
	var errs [2]error
	var wg sync.WaitGroup
	for i, event := range []order.Event{first, second} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results[i], errs[i] = store.ApplyEvent(ctx, order.SourceStream, event)
		}()
	}
	close(start)
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("concurrent ApplyEvent: %v", err)
		}
	}
	return results[0], results[1]
}

func assertLotState(
	ctx context.Context,
	t *testing.T,
	pool *pgxpool.Pool,
	clientOrderID order.ClientOrderID,
	wantRemaining, wantStatus string,
	wantClosedAt time.Time,
) {
	t.Helper()
	var remaining decimal.Decimal
	var status string
	var closedAt pgtype.Timestamptz
	if err := pool.QueryRow(ctx, `
SELECT l.remaining_qty, l.status, l.closed_at
FROM lots l JOIN fills f ON f.id=l.opened_by_fill_id
WHERE f.client_order_id=$1`, string(clientOrderID)).
		Scan(&remaining, &status, &closedAt); err != nil {
		t.Fatalf("query lot state: %v", err)
	}
	if !remaining.Equal(decimal.RequireFromString(wantRemaining)) || status != wantStatus {
		t.Fatalf("lot for order %s remaining=%s status=%s", clientOrderID, remaining, status)
	}
	if wantClosedAt.IsZero() && closedAt.Valid {
		t.Fatalf("open lot for order %s has closed_at %v", clientOrderID, closedAt.Time)
	}
	if !wantClosedAt.IsZero() && (!closedAt.Valid || !closedAt.Time.Equal(wantClosedAt)) {
		t.Fatalf("closed lot for order %s closed_at=%v valid=%v, want %v", clientOrderID, closedAt.Time, closedAt.Valid, wantClosedAt)
	}
}

func assertUnmatchedEvent(
	ctx context.Context,
	t *testing.T,
	pool *pgxpool.Pool,
	sell order.Request,
	want decimal.Decimal,
) {
	t.Helper()
	var fillID int64
	var qty decimal.Decimal
	if err := pool.QueryRow(ctx, `
SELECT sell_fill_id, qty FROM unmatched_sells
WHERE sell_fill_id=(SELECT id FROM fills WHERE client_order_id=$1)`, string(sell.ClientOrderID)).Scan(&fillID, &qty); err != nil {
		t.Fatalf("query unmatched sell: %v", err)
	}
	if !qty.Equal(want) {
		t.Fatalf("unmatched qty = %s, want %s", qty, want)
	}
	var body []byte
	if err := pool.QueryRow(ctx, `
SELECT payload FROM outbox
WHERE subject='ledger.unmatched_sell' AND payload->>'client_order_id'=$1`, string(sell.ClientOrderID)).Scan(&body); err != nil {
		t.Fatalf("query unmatched outbox: %v", err)
	}
	var payload unmatchedSellPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode unmatched payload: %v", err)
	}
	if payload.SellFillID != fillID || payload.UnmatchedQty != want.String() || payload.BotID != sell.BotID {
		t.Fatalf("unmatched payload = %+v", payload)
	}
	if got := countRows(ctx, t, pool, `
SELECT COUNT(*) FROM outbox
WHERE subject='ledger.unmatched_sell' AND payload->>'client_order_id'=$1`, string(sell.ClientOrderID)); got != 1 {
		t.Fatalf("unmatched outbox rows = %d, want 1", got)
	}
}

func assertLedgerCrossTableInvariant(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if got := countRows(ctx, t, pool, `
SELECT COUNT(*) FROM lots l
JOIN fills f ON f.id=l.opened_by_fill_id
JOIN orders o ON o.client_order_id=f.client_order_id
WHERE o.side <> 'buy' OR l.bot_id <> o.bot_id OR l.venue <> o.venue OR l.base <> o.base OR l.quote <> o.quote`); got != 0 {
		t.Fatalf("lots violating buy invariant = %d", got)
	}
	if got := countRows(ctx, t, pool, `
SELECT COUNT(*) FROM lot_closures c
JOIN fills f ON f.id=c.sell_fill_id
JOIN orders o ON o.client_order_id=f.client_order_id
JOIN lots l ON l.id=c.lot_id
WHERE o.side <> 'sell' OR l.bot_id <> o.bot_id OR l.venue <> o.venue OR l.base <> o.base OR l.quote <> o.quote`); got != 0 {
		t.Fatalf("closures violating sell invariant = %d", got)
	}
	if got := countRows(ctx, t, pool, `
SELECT COUNT(*) FROM unmatched_sells u
JOIN fills f ON f.id=u.sell_fill_id
JOIN orders o ON o.client_order_id=f.client_order_id
WHERE o.side <> 'sell' OR u.bot_id <> o.bot_id OR u.venue <> o.venue OR u.base <> o.base OR u.quote <> o.quote`); got != 0 {
		t.Fatalf("unmatched sells violating invariant = %d", got)
	}
}

func assertBuyOrderAccounting(
	ctx context.Context,
	t *testing.T,
	pool *pgxpool.Pool,
	clientOrderID order.ClientOrderID,
	want string,
) {
	t.Helper()
	var orderQty, fillQty, lotQty decimal.Decimal
	if err := pool.QueryRow(ctx, `
SELECT o.filled_qty,
       COALESCE((SELECT SUM(f.qty) FROM fills f WHERE f.client_order_id=o.client_order_id), 0),
       COALESCE((SELECT SUM(l.qty) FROM lots l JOIN fills f ON f.id=l.opened_by_fill_id
                 WHERE f.client_order_id=o.client_order_id), 0)
FROM orders o
WHERE o.client_order_id=$1`, string(clientOrderID)).Scan(&orderQty, &fillQty, &lotQty); err != nil {
		t.Fatalf("query buy accounting invariant: %v", err)
	}
	expected := decimal.RequireFromString(want)
	if !orderQty.Equal(expected) || !fillQty.Equal(expected) || !lotQty.Equal(expected) {
		t.Fatalf("accounting quantities: order=%s fills=%s lots=%s, want %s", orderQty, fillQty, lotQty, expected)
	}
}
