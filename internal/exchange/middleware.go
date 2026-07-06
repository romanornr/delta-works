// Package exchange composes venue connections: adapter → rate limiter →
// circuit breaker, registered in a Registry keyed by venue. Retries are NOT
// here. Retries belong to services, so every retry attempt passes back
// through the breaker and is counted by it (ADR-0003).
package exchange

import (
	"context"
	"errors"
	"fmt"

	"github.com/sony/gobreaker/v2"
	"golang.org/x/time/rate"

	"github.com/romanornr/delta-works/internal/domain/account"
	"github.com/romanornr/delta-works/internal/domain/instrument"
	"github.com/romanornr/delta-works/internal/domain/marketdata"
	"github.com/romanornr/delta-works/internal/domain/order"
	"github.com/romanornr/delta-works/internal/ports"
)

// WithRateLimit wraps an exchange so every call waits for the client-side
// limiter first. Venue rate limits are a property of the venue, enforced
// here; the adapter library's internal limiter is never the only guard.
func WithRateLimit(ex ports.Exchange, lim *rate.Limiter) ports.Exchange {
	return &rateLimited{ex: ex, lim: lim}
}

// errLimiterWait marks a failure to acquire the local rate limiter. The
// request never left the process, so the breaker must not count it against
// the venue.
var errLimiterWait = errors.New("local rate limiter wait")

type rateLimited struct {
	ex  ports.Exchange
	lim *rate.Limiter
}

func (r *rateLimited) ID() instrument.VenueID { return r.ex.ID() }

func (r *rateLimited) Ticker(ctx context.Context, inst instrument.Instrument) (marketdata.Ticker, error) {
	if err := r.lim.Wait(ctx); err != nil {
		return marketdata.Ticker{}, fmt.Errorf("%w: %w", errLimiterWait, err)
	}
	return r.ex.Ticker(ctx, inst)
}

func (r *rateLimited) Instruments(ctx context.Context, typ instrument.Type) ([]instrument.Instrument, error) {
	if err := r.lim.Wait(ctx); err != nil {
		return nil, fmt.Errorf("%w: %w", errLimiterWait, err)
	}
	return r.ex.Instruments(ctx, typ)
}

func (r *rateLimited) Balances(ctx context.Context, acct account.Type) ([]account.Balance, error) {
	if err := r.lim.Wait(ctx); err != nil {
		return nil, fmt.Errorf("%w: %w", errLimiterWait, err)
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

// Trading forwarding. The decorators pass order calls through the same
// limiter and breaker as the read path: a failing venue is failing as a
// venue. Adapters that cannot trade surface ports.ErrTradingUnsupported at
// call time.

func (r *rateLimited) PlaceOrder(ctx context.Context, req order.Request) (order.Ack, error) {
	op, ok := r.ex.(ports.OrderPlacer)
	if !ok {
		return order.Ack{}, fmt.Errorf("%w: %s", ports.ErrTradingUnsupported, r.ex.ID())
	}
	if err := r.lim.Wait(ctx); err != nil {
		return order.Ack{}, fmt.Errorf("%w: %w", errLimiterWait, err)
	}
	return op.PlaceOrder(ctx, req)
}

func (r *rateLimited) CancelOrder(ctx context.Context, ref order.Ref) error {
	op, ok := r.ex.(ports.OrderPlacer)
	if !ok {
		return fmt.Errorf("%w: %s", ports.ErrTradingUnsupported, r.ex.ID())
	}
	if err := r.lim.Wait(ctx); err != nil {
		return fmt.Errorf("%w: %w", errLimiterWait, err)
	}
	return op.CancelOrder(ctx, ref)
}

func (r *rateLimited) OpenOrders(ctx context.Context) ([]order.Snapshot, error) {
	op, ok := r.ex.(ports.OrderPlacer)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ports.ErrTradingUnsupported, r.ex.ID())
	}
	if err := r.lim.Wait(ctx); err != nil {
		return nil, fmt.Errorf("%w: %w", errLimiterWait, err)
	}
	return op.OpenOrders(ctx)
}

func (r *rateLimited) GetOrder(ctx context.Context, ref order.Ref) (order.Snapshot, error) {
	op, ok := r.ex.(ports.OrderPlacer)
	if !ok {
		return order.Snapshot{}, fmt.Errorf("%w: %s", ports.ErrTradingUnsupported, r.ex.ID())
	}
	if err := r.lim.Wait(ctx); err != nil {
		return order.Snapshot{}, fmt.Errorf("%w: %w", errLimiterWait, err)
	}
	return op.GetOrder(ctx, ref)
}

func (b *broken) PlaceOrder(ctx context.Context, req order.Request) (order.Ack, error) {
	op, ok := b.ex.(ports.OrderPlacer)
	if !ok {
		return order.Ack{}, fmt.Errorf("%w: %s", ports.ErrTradingUnsupported, b.ex.ID())
	}
	v, err := b.cb.Execute(func() (any, error) { return op.PlaceOrder(ctx, req) })
	if err != nil {
		return order.Ack{}, err
	}
	return v.(order.Ack), nil
}

func (b *broken) CancelOrder(ctx context.Context, ref order.Ref) error {
	op, ok := b.ex.(ports.OrderPlacer)
	if !ok {
		return fmt.Errorf("%w: %s", ports.ErrTradingUnsupported, b.ex.ID())
	}
	_, err := b.cb.Execute(func() (any, error) { return nil, op.CancelOrder(ctx, ref) })
	return err
}

func (b *broken) OpenOrders(ctx context.Context) ([]order.Snapshot, error) {
	op, ok := b.ex.(ports.OrderPlacer)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ports.ErrTradingUnsupported, b.ex.ID())
	}
	v, err := b.cb.Execute(func() (any, error) { return op.OpenOrders(ctx) })
	if err != nil {
		return nil, err
	}
	return v.([]order.Snapshot), nil
}

func (b *broken) GetOrder(ctx context.Context, ref order.Ref) (order.Snapshot, error) {
	op, ok := b.ex.(ports.OrderPlacer)
	if !ok {
		return order.Snapshot{}, fmt.Errorf("%w: %s", ports.ErrTradingUnsupported, b.ex.ID())
	}
	v, err := b.cb.Execute(func() (any, error) { return op.GetOrder(ctx, ref) })
	if err != nil {
		return order.Snapshot{}, err
	}
	return v.(order.Snapshot), nil
}
