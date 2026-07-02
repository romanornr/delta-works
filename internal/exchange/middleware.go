// Package exchange composes venue connections: adapter → rate limiter →
// circuit breaker, registered in a Registry keyed by venue. Retries are NOT
// here — they belong to services, so every retry attempt passes back
// through the breaker and is counted by it (ADR-0003).
package exchange

import (
	"context"

	"github.com/sony/gobreaker/v2"
	"golang.org/x/time/rate"

	"github.com/romanornr/delta-works/internal/domain/account"
	"github.com/romanornr/delta-works/internal/domain/instrument"
	"github.com/romanornr/delta-works/internal/domain/marketdata"
	"github.com/romanornr/delta-works/internal/ports"
)

// WithRateLimit wraps an exchange so every call waits for the client-side
// limiter first. Venue rate limits are a venue property enforced here — the
// adapter library's internal limiter is never the only guard.
func WithRateLimit(ex ports.Exchange, lim *rate.Limiter) ports.Exchange {
	return &rateLimited{ex: ex, lim: lim}
}

type rateLimited struct {
	ex  ports.Exchange
	lim *rate.Limiter
}

func (r *rateLimited) ID() instrument.VenueID { return r.ex.ID() }

func (r *rateLimited) Ticker(ctx context.Context, inst instrument.Instrument) (marketdata.Ticker, error) {
	if err := r.lim.Wait(ctx); err != nil {
		return marketdata.Ticker{}, err
	}
	return r.ex.Ticker(ctx, inst)
}

func (r *rateLimited) Instruments(ctx context.Context, typ instrument.Type) ([]instrument.Instrument, error) {
	if err := r.lim.Wait(ctx); err != nil {
		return nil, err
	}
	return r.ex.Instruments(ctx, typ)
}

func (r *rateLimited) Balances(ctx context.Context, acct account.Type) ([]account.Balance, error) {
	if err := r.lim.Wait(ctx); err != nil {
		return nil, err
	}
	return r.ex.Balances(ctx, acct)
}

// WithBreaker wraps an exchange with one circuit breaker covering all its
// calls: a venue that is failing is failing as a venue, not per endpoint.
func WithBreaker(ex ports.Exchange, settings gobreaker.Settings) ports.Exchange {
	return &broken{ex: ex, cb: gobreaker.NewCircuitBreaker[any](settings)}
}

type broken struct {
	ex ports.Exchange
	cb *gobreaker.CircuitBreaker[any]
}

func (b *broken) ID() instrument.VenueID { return b.ex.ID() }

func (b *broken) Ticker(ctx context.Context, inst instrument.Instrument) (marketdata.Ticker, error) {
	v, err := b.cb.Execute(func() (any, error) { return b.ex.Ticker(ctx, inst) })
	if err != nil {
		return marketdata.Ticker{}, err
	}
	return v.(marketdata.Ticker), nil
}

func (b *broken) Instruments(ctx context.Context, typ instrument.Type) ([]instrument.Instrument, error) {
	v, err := b.cb.Execute(func() (any, error) { return b.ex.Instruments(ctx, typ) })
	if err != nil {
		return nil, err
	}
	return v.([]instrument.Instrument), nil
}

func (b *broken) Balances(ctx context.Context, acct account.Type) ([]account.Balance, error) {
	v, err := b.cb.Execute(func() (any, error) { return b.ex.Balances(ctx, acct) })
	if err != nil {
		return nil, err
	}
	return v.([]account.Balance), nil
}
