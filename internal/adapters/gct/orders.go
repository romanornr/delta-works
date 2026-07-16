package gct

import (
	"context"
	"errors"
	"fmt"
	"time"

	gctorder "github.com/thrasher-corp/gocryptotrader/exchanges/order"

	"github.com/romanornr/delta-works/internal/domain/instrument"
	"github.com/romanornr/delta-works/internal/domain/order"
	"github.com/romanornr/delta-works/internal/ports"
)

var _ ports.OrderPlacer = (*Exchange)(nil)

// PlaceOrder implements ports.OrderPlacer. The ClientOrderID rides to the
// venue as the idempotency key; the caller has already persisted the
// pending row (docs/specs/manual-trading.md).
func (e *Exchange) PlaceOrder(ctx context.Context, req order.Request) (order.Ack, error) {
	submit, err := toGCTSubmit(e.exch.GetName(), e.exch, req)
	if err != nil {
		return order.Ack{}, err
	}
	resp, err := e.exch.SubmitOrder(ctx, submit)
	if err != nil {
		return order.Ack{}, fmt.Errorf("gct: submit order %s %s: %w", e.id, req.Instrument.Pair(), classify(err))
	}
	if resp == nil {
		// Third-party boundary: some venue wrappers are of uneven
		// quality, so a broken nil-with-no-error return is caught here.
		return order.Ack{}, fmt.Errorf("gct: submit order %s %s: nil response", e.id, req.Instrument.Pair())
	}

	status, err := toStatus(resp.Status)
	if err != nil {
		// The order exists at the venue; report acceptance and let the
		// stream or reconciliation settle the real status.
		status = order.StatusOpen
	}
	acceptedAt := resp.Date
	if acceptedAt.IsZero() {
		acceptedAt = time.Now().UTC()
	}
	return order.Ack{
		Ref: order.Ref{
			Instrument:    req.Instrument,
			ClientOrderID: req.ClientOrderID,
			VenueOrderID:  resp.OrderID,
		},
		Status:     status,
		AcceptedAt: acceptedAt,
	}, nil
}

// CancelOrder implements ports.OrderPlacer. Cancellation is an intent; the
// canceled state arrives like any other venue event.
func (e *Exchange) CancelOrder(ctx context.Context, ref order.Ref) error {
	cancel, err := toGCTCancel(e.exch.GetName(), e.exch, ref)
	if err != nil {
		return err
	}
	if err := e.exch.CancelOrder(ctx, cancel); err != nil {
		return fmt.Errorf("gct: cancel order %s %s: %w", e.id, ref.ClientOrderID, classify(err))
	}
	return nil
}

// OpenOrders implements ports.OrderPlacer. It is venue-wide by contract so
// reconciliation also sees orders placed outside this system.
func (e *Exchange) OpenOrders(ctx context.Context) ([]order.Snapshot, error) {
	item, err := toGCTAsset(instrument.TypeSpot)
	if err != nil {
		return nil, err
	}
	details, err := e.exch.GetActiveOrders(ctx, &gctorder.MultiOrderRequest{
		AssetType: item,
		Type:      gctorder.AnyType,
		Side:      gctorder.AnySide,
	})
	if err != nil {
		return nil, fmt.Errorf("gct: open orders %s: %w", e.id, classify(err))
	}

	out := make([]order.Snapshot, 0, len(details))
	for i := range details {
		snap, err := toSnapshot(e.id, &details[i])
		if err != nil {
			// Dropping the order would make reconciliation adopt a
			// terminal state for it; failing loud is the safe option.
			return nil, fmt.Errorf("gct: open orders %s: %w", e.id, err)
		}
		out = append(out, snap)
	}
	return out, nil
}

// GetOrder implements ports.OrderPlacer.
func (e *Exchange) GetOrder(ctx context.Context, ref order.Ref) (order.Snapshot, error) {
	if ref.VenueOrderID == "" {
		return order.Snapshot{}, fmt.Errorf(
			"gct: get order %s client %s: %w",
			e.id,
			ref.ClientOrderID,
			ports.ErrNoVenueOrderID,
		)
	}
	pair, item, err := toGCTPairAsset(e.exch, ref.Instrument)
	if err != nil {
		return order.Snapshot{}, err
	}
	detail, err := e.exch.GetOrderInfo(ctx, ref.VenueOrderID, pair, item)
	if errors.Is(err, gctorder.ErrOrderNotFound) {
		return order.Snapshot{}, fmt.Errorf("%w: %s %s: %w", ports.ErrNotFound, e.id, ref.VenueOrderID, err)
	}
	if err != nil {
		return order.Snapshot{}, fmt.Errorf("gct: get order %s %s: %w", e.id, ref.VenueOrderID, classify(err))
	}
	if detail == nil {
		return order.Snapshot{}, fmt.Errorf("%w: venue returned no order for %s %s", ports.ErrNotFound, e.id, ref.VenueOrderID)
	}
	snap, err := toSnapshot(e.id, detail)
	if err != nil {
		return order.Snapshot{}, fmt.Errorf("gct: get order %s %s: %w", e.id, ref.VenueOrderID, err)
	}
	return snap, nil
}
