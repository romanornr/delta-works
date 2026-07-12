// Package order holds the minimal order model. M1 compiles these types to
// lock the trading seam (ports.OrderPlacer); the full state machine,
// validation against instrument.Rules, and persistence arrive in M2.
package order

import (
	"time"

	"github.com/shopspring/decimal"

	"github.com/romanornr/delta-works/internal/domain/instrument"
	"github.com/romanornr/delta-works/internal/domain/money"
)

// ClientOrderID is OUR identifier, generated before submission. It is the
// idempotency key across placement retries and stream deduplication.
type ClientOrderID string

// Side of an order.
type Side string

// Order sides.
const (
	Buy  Side = "buy"
	Sell Side = "sell"
)

// Type of an order.
type Type string

// Order types. Algo types (iceberg, pegged, ...) are compositions in the
// execution layer, not venue order types.
const (
	Limit  Type = "limit"
	Market Type = "market"
)

// Status of an order at a venue.
type Status string

// Order statuses (the M2 state machine constrains transitions).
const (
	StatusPending         Status = "pending"
	StatusOpen            Status = "open"
	StatusPartiallyFilled Status = "partially_filled"
	StatusFilled          Status = "filled"
	StatusCanceled        Status = "canceled"
	StatusRejected        Status = "rejected"
	StatusExpired         Status = "expired"
)

// Request is an order to submit.
type Request struct {
	ClientOrderID ClientOrderID
	BotID         string // owning bot; control-plane orders use the reserved "manual"
	Instrument    instrument.Instrument
	Side          Side
	Type          Type
	Price         decimal.Decimal // zero for market orders
	Qty           decimal.Decimal
}

// Ref identifies an order at a venue.
type Ref struct {
	Instrument    instrument.Instrument
	ClientOrderID ClientOrderID
	VenueOrderID  string
}

// Ack is the venue's acceptance of a Request. Status is the state the
// venue reported at submit time; fills are never taken from the ack, they
// arrive through the stream or reconciliation.
type Ack struct {
	Ref        Ref
	Status     Status
	AcceptedAt time.Time
}

// Snapshot is a venue's current view of an order.
type Snapshot struct {
	Ref          Ref
	Status       Status
	Price        decimal.Decimal
	Qty          decimal.Decimal
	FilledQty    decimal.Decimal
	AvgFillPrice decimal.Decimal // venue's average fill price; zero when unknown
	UpdatedAt    time.Time
}

// Source identifies which path an event reached us through.
type Source string

// Event sources, persisted on every transition.
const (
	SourceLocal     Source = "local"
	SourceAck       Source = "ack"
	SourceStream    Source = "stream"
	SourceReconcile Source = "reconcile"
)

// Event is a change to an order reported by a venue. FilledQty is
// cumulative; the state machine derives per-fill deltas from it.
type Event struct {
	Ref         Ref
	Status      Status
	FilledQty   decimal.Decimal
	FillPrice   decimal.Decimal
	VenueFillID string // venue's fill identifier when provided; enables exact fill dedupe
	Fee         decimal.Decimal
	FeeCurrency money.Currency
	Reason      string // venue-provided reason for reject, cancel or expiry
	At          time.Time
}
