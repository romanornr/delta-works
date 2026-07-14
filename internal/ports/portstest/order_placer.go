// Package portstest contains reusable behavioral contracts for port adapters.
package portstest

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/romanornr/delta-works/internal/domain/instrument"
	"github.com/romanornr/delta-works/internal/domain/order"
	"github.com/romanornr/delta-works/internal/id"
	"github.com/romanornr/delta-works/internal/ports"
)

// Fixture declares venue capabilities and hard safety bounds for an order test.
type Fixture struct {
	Instrument          instrument.Instrument
	MinQty              decimal.Decimal
	MinNotional         decimal.Decimal
	NonMarketablePrice  func(context.Context) (decimal.Decimal, error)
	EchoesClientOrderID bool
	Deadline            time.Duration
	Cleanup             func(context.Context, ports.OrderPlacer, []order.Ref) error
}

// RunOrderPlacerContract verifies lookup, listing, and client-ID idempotency.
func RunOrderPlacerContract(t *testing.T, placer ports.OrderPlacer, fixture Fixture) {
	t.Helper()
	if fixture.Cleanup == nil || fixture.NonMarketablePrice == nil || fixture.Deadline <= 0 {
		t.Fatal("order placer fixture requires Cleanup, NonMarketablePrice, and a positive Deadline")
	}
	ctx, cancel := context.WithTimeout(t.Context(), fixture.Deadline)
	t.Cleanup(cancel)
	placed := make([]order.Ref, 0, 1)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), fixture.Deadline)
		defer cleanupCancel()
		if err := fixture.Cleanup(cleanupCtx, placer, placed); err != nil {
			t.Errorf("order contract cleanup left residue: %v", err)
		}
	})

	unknown := order.Ref{Instrument: fixture.Instrument, ClientOrderID: order.ClientOrderID(id.New()), VenueOrderID: "missing"}
	if _, err := placer.GetOrder(ctx, unknown); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("GetOrder(unknown) error = %v, want ports.ErrNotFound", err)
	}
	price, err := fixture.NonMarketablePrice(ctx)
	if err != nil {
		t.Fatalf("non-marketable price: %v", err)
	}
	qty := fixture.MinQty
	if price.IsPositive() && qty.Mul(price).LessThan(fixture.MinNotional) {
		qty = fixture.MinNotional.Div(price)
	}
	request := order.Request{
		ClientOrderID: order.ClientOrderID(id.New()), BotID: "contract",
		Instrument: fixture.Instrument, Side: order.Buy, Type: order.Limit,
		Price: price, Qty: qty,
	}
	ack, err := placer.PlaceOrder(ctx, request)
	if err != nil {
		t.Fatalf("first PlaceOrder: %v", err)
	}
	placed = append(placed, ack.Ref)
	if _, err := placer.PlaceOrder(ctx, request); err != nil {
		t.Logf("second PlaceOrder returned %v; identity is verified from venue state", err)
	}
	if fixture.EchoesClientOrderID && ack.Ref.ClientOrderID != request.ClientOrderID {
		t.Fatalf("ack client order ID = %q, want %q", ack.Ref.ClientOrderID, request.ClientOrderID)
	}
	snapshot, err := placer.GetOrder(ctx, ack.Ref)
	if err != nil {
		t.Fatalf("GetOrder(placed): %v", err)
	}
	if fixture.EchoesClientOrderID && snapshot.Ref.ClientOrderID != request.ClientOrderID {
		t.Fatalf("lookup client order ID = %q, want %q", snapshot.Ref.ClientOrderID, request.ClientOrderID)
	}
	open, err := placer.OpenOrders(ctx)
	if err != nil {
		t.Fatalf("OpenOrders: %v", err)
	}
	matches := 0
	for _, candidate := range open {
		if sameVenueOrder(candidate.Ref, ack.Ref, request.ClientOrderID) {
			matches++
		}
	}
	if matches != 1 {
		t.Fatalf("OpenOrders contains %d matching orders after duplicate placement, want 1", matches)
	}
}

func sameVenueOrder(got, want order.Ref, clientID order.ClientOrderID) bool {
	if want.VenueOrderID != "" {
		return got.VenueOrderID == want.VenueOrderID
	}
	return got.ClientOrderID == clientID
}

// CleanupPlacedOrders cancels every placed order and polls until none remain open.
func CleanupPlacedOrders(ctx context.Context, placer ports.OrderPlacer, refs []order.Ref) error {
	for _, ref := range refs {
		if err := placer.CancelOrder(ctx, ref); err != nil && !errors.Is(err, ports.ErrNotFound) {
			return fmt.Errorf("cancel %s: %w", ref.ClientOrderID, err)
		}
	}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		remaining := 0
		for _, ref := range refs {
			snapshot, err := placer.GetOrder(ctx, ref)
			switch {
			case errors.Is(err, ports.ErrNotFound):
			case err != nil:
				return fmt.Errorf("poll %s: %w", ref.ClientOrderID, err)
			case !snapshot.Status.Terminal():
				remaining++
			}
		}
		if remaining == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%d orders still open: %w", remaining, ctx.Err())
		case <-ticker.C:
		}
	}
}
