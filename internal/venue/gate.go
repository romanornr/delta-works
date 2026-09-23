package venue

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sony/gobreaker/v2"

	"github.com/romanornr/delta-works/internal/domain/account"
	"github.com/romanornr/delta-works/internal/domain/instrument"
	"github.com/romanornr/delta-works/internal/domain/marketdata"
	"github.com/romanornr/delta-works/internal/domain/order"
	"github.com/romanornr/delta-works/internal/ports"
)

var errLimiterWait = errors.New("local rate limiter wait")

type waiter interface {
	Wait(context.Context) error
}

type gate struct {
	waiter  waiter
	breaker *gobreaker.CircuitBreaker[any]
}

func newGate(id instrument.VenueID, waiter waiter) *gate {
	return &gate{waiter: waiter, breaker: gobreaker.NewCircuitBreaker[any](breakerSettings(id))}
}

func breakerSettings(id instrument.VenueID) gobreaker.Settings {
	return gobreaker.Settings{
		Name:         string(id),
		Timeout:      30 * time.Second,
		IsSuccessful: isBreakerSuccess,
	}
}

func isBreakerSuccess(err error) bool {
	return err == nil ||
		errors.Is(err, ports.ErrAuth) ||
		errors.Is(err, ports.ErrUnsupportedAccount) ||
		errors.Is(err, ports.ErrNotFound) ||
		errors.Is(err, ports.ErrNoVenueOrderID) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, errLimiterWait)
}

func guardedCall[T any](ctx context.Context, gate *gate, call func() (T, error)) (T, error) {
	value, err := gate.breaker.Execute(func() (any, error) {
		if err := gate.waiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("%w: %w", errLimiterWait, err)
		}
		return call()
	})
	if err != nil {
		var zero T
		return zero, err
	}
	return value.(T), nil
}

type guardedAccount struct {
	reader ports.AccountReader
	gate   *gate
}

func (g *guardedAccount) Balances(ctx context.Context, accountType account.Type) ([]account.Balance, error) {
	return guardedCall(ctx, g.gate, func() ([]account.Balance, error) { return g.reader.Balances(ctx, accountType) })
}

type guardedMarketData struct {
	reader ports.MarketDataReader
	gate   *gate
}

func (g *guardedMarketData) Ticker(ctx context.Context, inst instrument.Instrument) (marketdata.Ticker, error) {
	return guardedCall(ctx, g.gate, func() (marketdata.Ticker, error) { return g.reader.Ticker(ctx, inst) })
}

func (g *guardedMarketData) Instruments(ctx context.Context, typ instrument.Type) ([]instrument.Instrument, error) {
	return guardedCall(ctx, g.gate, func() ([]instrument.Instrument, error) { return g.reader.Instruments(ctx, typ) })
}

type guardedOrders struct {
	orders ports.OrderPlacer
	gate   *gate
}

func (g *guardedOrders) PlaceOrder(ctx context.Context, req order.Request) (order.Ack, error) {
	return guardedCall(ctx, g.gate, func() (order.Ack, error) { return g.orders.PlaceOrder(ctx, req) })
}

func (g *guardedOrders) CancelOrder(ctx context.Context, ref order.Ref) error {
	_, err := guardedCall(ctx, g.gate, func() (struct{}, error) { return struct{}{}, g.orders.CancelOrder(ctx, ref) })
	return err
}

func (g *guardedOrders) OpenOrders(ctx context.Context) ([]order.Snapshot, error) {
	return guardedCall(ctx, g.gate, func() ([]order.Snapshot, error) { return g.orders.OpenOrders(ctx) })
}

func (g *guardedOrders) GetOrder(ctx context.Context, ref order.Ref) (order.Snapshot, error) {
	return guardedCall(ctx, g.gate, func() (order.Snapshot, error) { return g.orders.GetOrder(ctx, ref) })
}
