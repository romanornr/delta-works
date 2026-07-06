package gct

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/thrasher-corp/gocryptotrader/currency"
	gctorder "github.com/thrasher-corp/gocryptotrader/exchanges/order"

	"github.com/romanornr/delta-works/internal/domain/instrument"
	"github.com/romanornr/delta-works/internal/domain/order"
)

func orderPairInst(t *testing.T) (currency.Pair, instrument.Instrument) {
	t.Helper()
	pair, err := currency.NewPairFromStrings("BTC", "USDT")
	if err != nil {
		t.Fatal(err)
	}
	inst := instrument.Instrument{
		Venue: "bybit", Type: instrument.TypeSpot,
		Base: "BTC", Quote: "USDT", VenueSymbol: "BTCUSDT",
	}
	return pair, inst
}

func TestToGCTSubmit(t *testing.T) {
	pair, inst := orderPairInst(t)
	req := order.Request{
		ClientOrderID: "01ARZ3NDEKTSV4RRFFQ69G5FAV",
		BotID:         "manual",
		Instrument:    inst,
		Side:          order.Buy,
		Type:          order.Limit,
		Price:         decimal.RequireFromString("50000.5"),
		Qty:           decimal.RequireFromString("0.25"),
	}
	got, err := toGCTSubmit("bybit", &fakeMatcher{pair: pair}, req)
	if err != nil {
		t.Fatalf("toGCTSubmit: %v", err)
	}
	if got.Exchange != "bybit" || got.ClientOrderID != "01ARZ3NDEKTSV4RRFFQ69G5FAV" {
		t.Fatalf("identity fields = %q %q", got.Exchange, got.ClientOrderID)
	}
	if got.Side != gctorder.Buy || got.Type != gctorder.Limit {
		t.Fatalf("side/type = %v %v", got.Side, got.Type)
	}
	if got.Price != 50000.5 || got.Amount != 0.25 {
		t.Fatalf("price/amount = %v %v", got.Price, got.Amount)
	}

	req.Side = "short"
	if _, err := toGCTSubmit("bybit", &fakeMatcher{pair: pair}, req); err == nil {
		t.Fatal("unsupported side: want error")
	}
	req.Side = order.Sell
	req.Type = "iceberg"
	if _, err := toGCTSubmit("bybit", &fakeMatcher{pair: pair}, req); err == nil {
		t.Fatal("unsupported type: want error")
	}
}

func TestToGCTCancel(t *testing.T) {
	pair, inst := orderPairInst(t)
	got, err := toGCTCancel("bybit", &fakeMatcher{pair: pair}, order.Ref{
		Instrument: inst, ClientOrderID: "cid-1", VenueOrderID: "v-1",
	})
	if err != nil {
		t.Fatalf("toGCTCancel: %v", err)
	}
	if got.OrderID != "v-1" || got.ClientOrderID != "cid-1" || got.Exchange != "bybit" {
		t.Fatalf("cancel = %+v", got)
	}
}

func TestToStatusTotal(t *testing.T) {
	// Every GCT status either maps or errors; nothing may fall through to
	// a wrong default. The unmappable ones are the wildcard queries.
	want := map[gctorder.Status]order.Status{
		gctorder.Pending:                  order.StatusPending,
		gctorder.New:                      order.StatusOpen,
		gctorder.Active:                   order.StatusOpen,
		gctorder.Open:                     order.StatusOpen,
		gctorder.Hidden:                   order.StatusOpen,
		gctorder.PendingCancel:            order.StatusOpen,
		gctorder.Cancelling:               order.StatusOpen,
		gctorder.PartiallyFilled:          order.StatusPartiallyFilled,
		gctorder.Filled:                   order.StatusFilled,
		gctorder.Closed:                   order.StatusFilled,
		gctorder.Cancelled:                order.StatusCanceled,
		gctorder.PartiallyCancelled:       order.StatusCanceled,
		gctorder.PartiallyFilledCancelled: order.StatusCanceled,
		gctorder.Liquidated:               order.StatusCanceled,
		gctorder.AutoDeleverage:           order.StatusCanceled,
		gctorder.Rejected:                 order.StatusRejected,
		gctorder.InsufficientBalance:      order.StatusRejected,
		gctorder.MarketUnavailable:        order.StatusRejected,
		gctorder.STP:                      order.StatusRejected,
		gctorder.Expired:                  order.StatusExpired,
	}
	for gctStatus, wantStatus := range want {
		got, err := toStatus(gctStatus)
		if err != nil || got != wantStatus {
			t.Errorf("toStatus(%v) = %q, %v; want %q", gctStatus, got, err, wantStatus)
		}
	}
	for _, unmappable := range []gctorder.Status{gctorder.UnknownStatus, gctorder.AnyStatus} {
		if _, err := toStatus(unmappable); err == nil {
			t.Errorf("toStatus(%v): want error", unmappable)
		}
	}
}

func detail(t *testing.T, trades []gctorder.TradeHistory) *gctorder.Detail {
	t.Helper()
	pair, _ := orderPairInst(t)
	return &gctorder.Detail{
		Exchange:             "bybit",
		OrderID:              "v-1",
		ClientOrderID:        "cid-1",
		Pair:                 pair,
		Status:               gctorder.PartiallyFilled,
		Price:                50000,
		Amount:               1,
		ExecutedAmount:       0.4,
		AverageExecutedPrice: 49900,
		LastUpdated:          time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC),
		Trades:               trades,
	}
}

func TestToEventSingleTradeCarriesFillFacts(t *testing.T) {
	ev, err := toEvent("bybit", detail(t, []gctorder.TradeHistory{
		{TID: "f-1", Price: 49900, Amount: 0.4, Fee: 0.01, FeeAsset: "USDT"},
	}))
	if err != nil {
		t.Fatalf("toEvent: %v", err)
	}
	if ev.VenueFillID != "f-1" || !ev.FillPrice.Equal(decimal.NewFromInt(49900)) {
		t.Fatalf("fill facts = %q %s", ev.VenueFillID, ev.FillPrice)
	}
	if !ev.Fee.Equal(decimal.RequireFromString("0.01")) || ev.FeeCurrency != "USDT" {
		t.Fatalf("fee = %s %s", ev.Fee, ev.FeeCurrency)
	}
	if ev.Status != order.StatusPartiallyFilled || !ev.FilledQty.Equal(decimal.RequireFromString("0.4")) {
		t.Fatalf("status/cumulative = %s %s", ev.Status, ev.FilledQty)
	}
	if ev.Ref.ClientOrderID != "cid-1" || ev.Ref.VenueOrderID != "v-1" {
		t.Fatalf("ref = %+v", ev.Ref)
	}
}

func TestToEventMultipleTradesFallsBackToAverage(t *testing.T) {
	ev, err := toEvent("bybit", detail(t, []gctorder.TradeHistory{
		{TID: "f-1", Price: 49800, Amount: 0.2},
		{TID: "f-2", Price: 50000, Amount: 0.2},
	}))
	if err != nil {
		t.Fatalf("toEvent: %v", err)
	}
	if ev.VenueFillID != "" || !ev.Fee.IsZero() {
		t.Fatalf("ambiguous trades must not carry fill facts: %q %s", ev.VenueFillID, ev.Fee)
	}
	if !ev.FillPrice.Equal(decimal.NewFromInt(49900)) {
		t.Fatalf("FillPrice = %s, want average 49900", ev.FillPrice)
	}
}

func TestToSnapshot(t *testing.T) {
	snap, err := toSnapshot("bybit", detail(t, nil))
	if err != nil {
		t.Fatalf("toSnapshot: %v", err)
	}
	if snap.Status != order.StatusPartiallyFilled || !snap.FilledQty.Equal(decimal.RequireFromString("0.4")) {
		t.Fatalf("snapshot = %+v", snap)
	}
	if snap.Ref.Instrument.VenueSymbol == "" || snap.Ref.Instrument.Venue != "bybit" {
		t.Fatalf("instrument = %+v", snap.Ref.Instrument)
	}
}

func TestToOrderEvents(t *testing.T) {
	single := detail(t, nil)
	batch := []gctorder.Detail{*detail(t, nil), *detail(t, nil)}
	unmappable := detail(t, nil)
	unmappable.Status = gctorder.UnknownStatus

	if got := toOrderEvents("bybit", single); len(got) != 1 {
		t.Fatalf("pointer detail: %d events, want 1", len(got))
	}
	if got := toOrderEvents("bybit", batch); len(got) != 2 {
		t.Fatalf("detail slice: %d events, want 2", len(got))
	}
	if got := toOrderEvents("bybit", unmappable); len(got) != 0 {
		t.Fatalf("unmappable status: %d events, want 0", len(got))
	}
	if got := toOrderEvents("bybit", "ticker noise"); got != nil {
		t.Fatalf("non-order payload: %v, want nil", got)
	}
	if got := toOrderEvents("bybit", (*gctorder.Detail)(nil)); got != nil {
		t.Fatalf("typed-nil detail: %v, want nil", got)
	}
}
