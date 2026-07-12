package order

import (
	"time"

	"github.com/shopspring/decimal"

	"github.com/romanornr/delta-works/internal/domain/instrument"
	"github.com/romanornr/delta-works/internal/domain/money"
)

// Bus subjects for order events. Order events are published only through
// the Postgres outbox (ADR-0008), never with a direct bus publish.
const (
	SubjectUpdated = "order.updated"
	SubjectFilled  = "order.filled"
)

// UpdatedPayload is the outbox payload for SubjectUpdated.
type UpdatedPayload struct {
	ClientOrderID ClientOrderID      `json:"client_order_id"`
	Venue         instrument.VenueID `json:"venue"`
	Base          money.Currency     `json:"base"`
	Quote         money.Currency     `json:"quote"`
	Status        Status             `json:"status"`
	FilledQty     decimal.Decimal    `json:"filled_qty"`
	Source        Source             `json:"source"`
	At            time.Time          `json:"at"`
}

// FilledPayload is the outbox payload for SubjectFilled. Qty is the fill
// delta, not the cumulative quantity.
type FilledPayload struct {
	ClientOrderID ClientOrderID      `json:"client_order_id"`
	Venue         instrument.VenueID `json:"venue"`
	Base          money.Currency     `json:"base"`
	Quote         money.Currency     `json:"quote"`
	Status        Status             `json:"status"`
	FilledQty     decimal.Decimal    `json:"filled_qty"`
	Qty           decimal.Decimal    `json:"qty"`
	Price         decimal.Decimal    `json:"price"`
	Fee           decimal.Decimal    `json:"fee"`
	FeeCurrency   money.Currency     `json:"fee_currency,omitempty"`
	VenueFillID   string             `json:"venue_fill_id,omitempty"`
	At            time.Time          `json:"at"`
}
