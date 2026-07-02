// Package marketdata models market observations.
package marketdata

import (
	"time"

	"github.com/shopspring/decimal"

	"github.com/romanornr/delta-works/internal/domain/instrument"
)

// Ticker is a top-of-book plus last-trade observation.
type Ticker struct {
	Instrument instrument.Instrument
	Bid        decimal.Decimal
	Ask        decimal.Decimal
	Last       decimal.Decimal
	BidSize    decimal.Decimal
	AskSize    decimal.Decimal
	At         time.Time
}
