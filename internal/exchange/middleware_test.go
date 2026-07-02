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
