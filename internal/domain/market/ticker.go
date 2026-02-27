package market

import (
	"time"

	"github.com/shopspring/decimal"
)

// Ticker represents current market data for a trading pair
type Ticker struct {
	Exchange  string          `json:"exchange"`
	Base      string          `json:"base"`
	Quote     string          `json:"quote"`
	Last      decimal.Decimal `json:"last"`
	Timestamp time.Time       `json:"timestamp"`
	Top       *TopOfBook      `json:"top,omitempty"`
	Candle    *Candle         `json:"candle,omitempty"`
}

// NewTicker creates a new ticker
func NewTicker(exchange, base, quote string, last decimal.Decimal, ts time.Time) *Ticker {
	return &Ticker{
		Exchange:  exchange,
		Base:      base,
		Quote:     quote,
		Last:      last,
		Timestamp: ts,
	}
}

// Pair returns the trading pair as "BASE/QUOTE"
func (t *Ticker) Pair() string {
	return t.Base + "/" + t.Quote
}

// IsValid reports whether the ticker has required fields
func (t *Ticker) IsValid() bool {
	return t.Exchange != "" && t.Base != "" && t.Quote != "" && !t.Last.IsZero()
}
