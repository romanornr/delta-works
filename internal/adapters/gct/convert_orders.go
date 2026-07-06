package gct

// Order conversions. GCT carries order prices and quantities as float64;
// our requests convert decimal to float64 at this edge (exact for values
// that fit a float64, which quantized order prices do), and venue reports
// convert back through decimal.NewFromFloat. Exactness is bounded by GCT's
// types until native adapters replace it per venue (ADR-0003).

import (
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"github.com/thrasher-corp/gocryptotrader/currency"
	gctorder "github.com/thrasher-corp/gocryptotrader/exchanges/order"

	"github.com/romanornr/delta-works/internal/domain/instrument"
	"github.com/romanornr/delta-works/internal/domain/money"
	"github.com/romanornr/delta-works/internal/domain/order"
)

func toGCTSubmit(name string, m symbolMatcher, req order.Request) (*gctorder.Submit, error) {
	pair, item, err := toGCTPairAsset(m, req.Instrument)
	if err != nil {
		return nil, err
	}
	side, err := toGCTSide(req.Side)
	if err != nil {
		return nil, err
	}
	typ, err := toGCTType(req.Type)
	if err != nil {
		return nil, err
	}
	return &gctorder.Submit{
		Exchange:      name,
		Pair:          pair,
		AssetType:     item,
		Side:          side,
		Type:          typ,
		Price:         req.Price.InexactFloat64(),
		Amount:        req.Qty.InexactFloat64(),
		ClientOrderID: string(req.ClientOrderID),
	}, nil
}

func toGCTCancel(name string, m symbolMatcher, ref order.Ref) (*gctorder.Cancel, error) {
	pair, item, err := toGCTPairAsset(m, ref.Instrument)
	if err != nil {
		return nil, err
	}
	return &gctorder.Cancel{
		Exchange:      name,
		OrderID:       ref.VenueOrderID,
		ClientOrderID: string(ref.ClientOrderID),
		Pair:          pair,
		AssetType:     item,
	}, nil
}

func toGCTSide(s order.Side) (gctorder.Side, error) {
	switch s {
	case order.Buy:
		return gctorder.Buy, nil
	case order.Sell:
		return gctorder.Sell, nil
	default:
		return gctorder.UnknownSide, fmt.Errorf("gct: order side %q not supported", s)
	}
}

func toGCTType(t order.Type) (gctorder.Type, error) {
	switch t {
	case order.Limit:
		return gctorder.Limit, nil
	case order.Market:
		return gctorder.Market, nil
	default:
		return gctorder.UnknownType, fmt.Errorf("gct: order type %q not supported", t)
	}
}

// toStatus maps every GCT status onto the domain state machine. Cancel-in-
// flight statuses map to open because cancel is an intent, not a state
// (docs/specs/m2-oms.md); forced closes (liquidation, ADL) map to canceled.
func toStatus(s gctorder.Status) (order.Status, error) {
	switch s {
	case gctorder.Pending:
		return order.StatusPending, nil
	case gctorder.New, gctorder.Active, gctorder.Open, gctorder.Hidden,
		gctorder.PendingCancel, gctorder.Cancelling:
		return order.StatusOpen, nil
	case gctorder.PartiallyFilled:
		return order.StatusPartiallyFilled, nil
	case gctorder.Filled, gctorder.Closed:
		return order.StatusFilled, nil
	case gctorder.Cancelled, gctorder.PartiallyCancelled,
		gctorder.PartiallyFilledCancelled, gctorder.Liquidated, gctorder.AutoDeleverage:
		return order.StatusCanceled, nil
	case gctorder.Rejected, gctorder.InsufficientBalance,
		gctorder.MarketUnavailable, gctorder.STP:
		return order.StatusRejected, nil
	case gctorder.Expired:
		return order.StatusExpired, nil
	default:
		return "", fmt.Errorf("gct: order status %q not mappable", s)
	}
}

func fromGCTPair(venue instrument.VenueID, pair currency.Pair) instrument.Instrument {
	return instrument.Instrument{
		Venue:       venue,
		Type:        instrument.TypeSpot,
		Base:        money.NewCurrency(pair.Base.String()),
		Quote:       money.NewCurrency(pair.Quote.String()),
		VenueSymbol: pair.String(),
	}
}

func toRef(venue instrument.VenueID, d *gctorder.Detail) order.Ref {
	return order.Ref{
		Instrument:    fromGCTPair(venue, d.Pair),
		ClientOrderID: order.ClientOrderID(d.ClientOrderID),
		VenueOrderID:  d.OrderID,
	}
}

func toSnapshot(venue instrument.VenueID, d *gctorder.Detail) (order.Snapshot, error) {
	status, err := toStatus(d.Status)
	if err != nil {
		return order.Snapshot{}, err
	}
	return order.Snapshot{
		Ref:       toRef(venue, d),
		Status:    status,
		Price:     decimal.NewFromFloat(d.Price),
		Qty:       decimal.NewFromFloat(d.Amount),
		FilledQty: decimal.NewFromFloat(d.ExecutedAmount),
		UpdatedAt: detailTime(d),
	}, nil
}

// toEvent flattens a venue Detail into one domain event. Per-fill facts
// (price, fee, venue fill ID) are taken from the attached trade only when
// exactly one is present; with several trades the split of the cumulative
// delta is ambiguous, so the event falls back to the average executed
// price and carries no fill ID or fee. The state machine only needs the
// cumulative quantity either way.
func toEvent(venue instrument.VenueID, d *gctorder.Detail) (order.Event, error) {
	status, err := toStatus(d.Status)
	if err != nil {
		return order.Event{}, err
	}
	ev := order.Event{
		Ref:       toRef(venue, d),
		Status:    status,
		FilledQty: decimal.NewFromFloat(d.ExecutedAmount),
		FillPrice: decimal.NewFromFloat(d.AverageExecutedPrice),
		At:        detailTime(d),
	}
	if len(d.Trades) == 1 {
		t := d.Trades[0]
		ev.VenueFillID = t.TID
		ev.FillPrice = decimal.NewFromFloat(t.Price)
		ev.Fee = decimal.NewFromFloat(t.Fee)
		ev.FeeCurrency = money.NewCurrency(t.FeeAsset)
	}
	return ev, nil
}

func detailTime(d *gctorder.Detail) time.Time {
	switch {
	case !d.LastUpdated.IsZero():
		return d.LastUpdated
	case !d.Date.IsZero():
		return d.Date
	default:
		return time.Now().UTC()
	}
}
