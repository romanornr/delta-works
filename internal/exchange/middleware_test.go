package exchange

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sony/gobreaker/v2"
	"golang.org/x/time/rate"

	"github.com/romanornr/delta-works/internal/domain/account"
	"github.com/romanornr/delta-works/internal/domain/instrument"
	"github.com/romanornr/delta-works/internal/domain/marketdata"
	"github.com/romanornr/delta-works/internal/domain/order"
	"github.com/romanornr/delta-works/internal/ports"
)

type fakeExchange struct {
	id    instrument.VenueID
	calls int
	err   error
}

func (f *fakeExchange) ID() instrument.VenueID { return f.id }

func (f *fakeExchange) Ticker(context.Context, instrument.Instrument) (marketdata.Ticker, error) {
	f.calls++
	return marketdata.Ticker{}, f.err
}

func (f *fakeExchange) Instruments(context.Context, instrument.Type) ([]instrument.Instrument, error) {
	f.calls++
	return nil, f.err
}

func (f *fakeExchange) Balances(context.Context, account.Type) ([]account.Balance, error) {
	f.calls++
	return []account.Balance{}, f.err
}

func TestWithRateLimitWaits(t *testing.T) {
	fake := &fakeExchange{id: "x"}
	// 100 rps, burst 1: second call must wait ~10ms.
	ex := WithRateLimit(fake, rate.NewLimiter(100, 1))

	start := time.Now()
	for range 2 {
		if _, err := ex.Balances(context.Background(), account.TypeSpot); err != nil {
			t.Fatal(err)
		}
	}
	if elapsed := time.Since(start); elapsed < 5*time.Millisecond {
		t.Errorf("expected rate limiting delay, got %s", elapsed)
	}
	if fake.calls != 2 {
		t.Errorf("calls: got %d", fake.calls)
	}
}

func TestWithRateLimitHonorsContextCancel(t *testing.T) {
	fake := &fakeExchange{id: "x"}
	ex := WithRateLimit(fake, rate.NewLimiter(rate.Limit(0.001), 1))

	_, _ = ex.Balances(context.Background(), account.TypeSpot) // consume burst
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := ex.Balances(ctx, account.TypeSpot); err == nil {
		t.Error("expected context error while waiting for limiter")
	}
	if fake.calls != 1 {
		t.Errorf("underlying called despite canceled wait: %d", fake.calls)
	}
}

func TestWithBreakerOpensAfterFailures(t *testing.T) {
	fake := &fakeExchange{id: "x", err: errors.New("venue down")}
	ex := WithBreaker(fake, gobreaker.Settings{
		Name: "x",
		ReadyToTrip: func(c gobreaker.Counts) bool {
			return c.ConsecutiveFailures >= 3
		},
	})

	for range 5 {
		_, _ = ex.Ticker(context.Background(), instrument.Instrument{})
	}

	if fake.calls != 3 {
		t.Errorf("expected breaker to stop calls after 3 failures, underlying saw %d", fake.calls)
	}
	if _, err := ex.Ticker(context.Background(), instrument.Instrument{}); !errors.Is(err, gobreaker.ErrOpenState) {
		t.Errorf("expected ErrOpenState, got %v", err)
	}
}

func TestBreakerIgnoresNonVenueErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, true},
		{"auth", ports.ErrAuth, true},
		{"unsupported account", ports.ErrUnsupportedAccount, true},
		{"order not found", ports.ErrNotFound, true},
		{"missing venue order ID", ports.ErrNoVenueOrderID, true},
		{"caller canceled", context.Canceled, true},
		{"limiter wait timeout", errLimiterWait, true},
		{"venue failure", errors.New("venue down"), false},
		{"venue deadline", context.DeadlineExceeded, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isBreakerSuccess(tt.err); got != tt.want {
				t.Errorf("isBreakerSuccess(%v): got %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestDecoratedLimiterTimeoutDoesNotTripBreaker(t *testing.T) {
	fake := &fakeExchange{id: "x"}
	ex := Decorate(fake, 0.001, 1)

	if _, err := ex.Balances(context.Background(), account.TypeSpot); err != nil {
		t.Fatal(err) // consume the burst token
	}
	for range 10 {
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		_, _ = ex.Balances(ctx, account.TypeSpot)
		cancel()
	}
	// An open breaker rejects before the limiter runs, so a deadline on the
	// probe keeps it from blocking on the drained limiter when it passes.
	probeCtx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	if _, err := ex.Balances(probeCtx, account.TypeSpot); errors.Is(err, gobreaker.ErrOpenState) {
		t.Error("limiter wait timeouts opened the venue breaker")
	}
	if fake.calls != 1 {
		t.Errorf("underlying called during limiter waits: %d", fake.calls)
	}
}

func TestRegistry(t *testing.T) {
	a, b := &fakeExchange{id: "a"}, &fakeExchange{id: "b"}
	r := NewRegistry([]ports.Exchange{a, b, &fakeExchange{id: "a"}})

	if got, err := r.Get("a"); err != nil || got.ID() != "a" {
		t.Errorf("Get(a): %v %v", got, err)
	}
	if _, err := r.Get("missing"); err == nil {
		t.Error("expected error for unknown venue")
	}
	if len(r.All()) != 2 {
		t.Errorf("duplicate registration not ignored: %d", len(r.All()))
	}
}

type fakeTradingExchange struct {
	fakeExchange
	placeCalls int
}

func (f *fakeTradingExchange) PlaceOrder(_ context.Context, req order.Request) (order.Ack, error) {
	f.placeCalls++
	return order.Ack{Ref: order.Ref{ClientOrderID: req.ClientOrderID}}, f.err
}
func (f *fakeTradingExchange) CancelOrder(context.Context, order.Ref) error { return f.err }
func (f *fakeTradingExchange) OpenOrders(context.Context) ([]order.Snapshot, error) {
	return nil, f.err
}

func (f *fakeTradingExchange) GetOrder(context.Context, order.Ref) (order.Snapshot, error) {
	return order.Snapshot{}, f.err
}

func TestDecoratorsForwardTrading(t *testing.T) {
	t.Parallel()

	fake := &fakeTradingExchange{fakeExchange: fakeExchange{id: "bybit"}}
	decorated := Decorate(fake, 100, 100)

	placer, ok := decorated.(ports.OrderPlacer)
	if !ok {
		t.Fatal("decorated exchange must expose ports.OrderPlacer")
	}
	ack, err := placer.PlaceOrder(context.Background(), order.Request{ClientOrderID: "cid-1"})
	if err != nil || ack.Ref.ClientOrderID != "cid-1" || fake.placeCalls != 1 {
		t.Fatalf("PlaceOrder = %+v, %v, calls=%d", ack, err, fake.placeCalls)
	}
	if err := placer.CancelOrder(context.Background(), order.Ref{}); err != nil {
		t.Fatalf("CancelOrder: %v", err)
	}
	if _, err := placer.OpenOrders(context.Background()); err != nil {
		t.Fatalf("OpenOrders: %v", err)
	}
	if _, err := placer.GetOrder(context.Background(), order.Ref{}); err != nil {
		t.Fatalf("GetOrder: %v", err)
	}
}

func TestDecoratorsRejectNonTradingAdapter(t *testing.T) {
	t.Parallel()

	decorated := Decorate(&fakeExchange{id: "bybit"}, 100, 100)
	placer, ok := decorated.(ports.OrderPlacer)
	if !ok {
		t.Fatal("decorated exchange must expose ports.OrderPlacer")
	}
	if _, err := placer.PlaceOrder(context.Background(), order.Request{}); !errors.Is(err, ports.ErrTradingUnsupported) {
		t.Fatalf("PlaceOrder err = %v, want ErrTradingUnsupported", err)
	}
	if _, err := placer.OpenOrders(context.Background()); !errors.Is(err, ports.ErrTradingUnsupported) {
		t.Fatalf("OpenOrders err = %v, want ErrTradingUnsupported", err)
	}
}
