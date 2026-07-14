// Package instrument identifies tradable instruments across venues.
package instrument

import (
	"fmt"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/romanornr/delta-works/internal/domain/money"
)

// VenueID is a canonical lowercase venue name, e.g. "bybit".
type VenueID string

// NewVenueID canonicalizes a venue name to lowercase.
func NewVenueID(name string) VenueID {
	return VenueID(strings.ToLower(strings.TrimSpace(name)))
}

// Type is the instrument class. Only spot exists today; futures/perps later.
type Type string

// TypeSpot is the spot instrument class.
const TypeSpot Type = "spot"

// Instrument carries both canonical identity (Base/Quote/Type) and the
// venue-native symbol, so adapters never re-derive symbols.
type Instrument struct {
	Venue       VenueID
	Type        Type
	Base, Quote money.Currency
	// VenueSymbol is the venue's native symbol, e.g. "BTCUSDT". Adapters
	// use it as the round-trip key when talking to the venue.
	VenueSymbol string
	// Rules are the venue's trading constraints. Optional for snapshots; required
	// by order validation.
	Rules Rules
}

// Rules are venue trading constraints for an instrument.
type Rules struct {
	PriceIncrement decimal.Decimal
	QtyIncrement   decimal.Decimal
	MinQty         decimal.Decimal
	MinNotional    decimal.Decimal
}

// Key returns the canonical map key, e.g. "bybit:spot:BTC/USDT".
func (i Instrument) Key() string {
	return fmt.Sprintf("%s:%s:%s/%s", i.Venue, i.Type, i.Base, i.Quote)
}

// Pair returns the canonical pair string, e.g. "BTC/USDT".
func (i Instrument) Pair() string {
	return string(i.Base) + "/" + string(i.Quote)
}

// ParsePair splits a canonical "BASE/QUOTE" pair string.
func ParsePair(s string) (base, quote money.Currency, err error) {
	left, right, found := strings.Cut(s, "/")
	if !found {
		return "", "", fmt.Errorf("parse pair %q: missing '/'", s)
	}
	base, quote = money.NewCurrency(left), money.NewCurrency(right)
	if base == "" || quote == "" {
		return "", "", fmt.Errorf("parse pair %q: empty base or quote", s)
	}
	if strings.Contains(string(quote), "/") {
		return "", "", fmt.Errorf("parse pair %q: more than one '/'", s)
	}
	return base, quote, nil
}
