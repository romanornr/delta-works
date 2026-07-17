// Package events owns internal event subjects and their payloads.
package events

import (
	"time"

	"github.com/shopspring/decimal"

	"github.com/romanornr/delta-works/internal/domain/instrument"
	"github.com/romanornr/delta-works/internal/domain/money"
	"github.com/romanornr/delta-works/internal/domain/order"
)

// Order subjects are outbox-only under ADR-0008; the orphan subject remains a bus hint.
const (
	SubjectOrderUpdated    = "order.updated"
	SubjectOrderFilled     = "order.filled"
	SubjectReconcileOrphan = "reconcile.orphan"
)

// OrderUpdatedPayload is the outbox payload for SubjectOrderUpdated.
type OrderUpdatedPayload struct {
	ClientOrderID order.ClientOrderID `json:"client_order_id"`
	Venue         instrument.VenueID  `json:"venue"`
	Base          money.Currency      `json:"base"`
	Quote         money.Currency      `json:"quote"`
	Status        order.Status        `json:"status"`
	FilledQty     decimal.Decimal     `json:"filled_qty"`
	Source        order.Source        `json:"source"`
	At            time.Time           `json:"at"`
}

// OrderFilledPayload is the outbox payload for SubjectOrderFilled; Qty is a delta, not a cumulative value.
type OrderFilledPayload struct {
	ClientOrderID order.ClientOrderID `json:"client_order_id"`
	Venue         instrument.VenueID  `json:"venue"`
	Base          money.Currency      `json:"base"`
	Quote         money.Currency      `json:"quote"`
	Status        order.Status        `json:"status"`
	FilledQty     decimal.Decimal     `json:"filled_qty"`
	Qty           decimal.Decimal     `json:"qty"`
	Price         decimal.Decimal     `json:"price"`
	Fee           decimal.Decimal     `json:"fee"`
	FeeCurrency   money.Currency      `json:"fee_currency,omitempty"`
	VenueFillID   string              `json:"venue_fill_id,omitempty"`
	At            time.Time           `json:"at"`
}

// ReconcileOrphanPayload reports an unknown venue order that must not be adopted automatically.
type ReconcileOrphanPayload struct {
	Venue         instrument.VenueID
	VenueOrderID  string
	ClientOrderID order.ClientOrderID
	Base, Quote   string
}

// OutboxMessage is one unpublished transactional-outbox row.
type OutboxMessage struct {
	ID        int64
	Subject   string
	Payload   []byte // jsonb
	CreatedAt time.Time
}
