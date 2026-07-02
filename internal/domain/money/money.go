// Package money provides exact currency-tagged amounts. Domain packages are
// pure: stdlib + shopspring/decimal only. float64 never appears here.
package money

import (
	"errors"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

// ErrCurrencyMismatch is returned when combining amounts of different currencies.
var ErrCurrencyMismatch = errors.New("currency mismatch")

// Currency is a canonical uppercase asset code, e.g. "BTC", "USDT".
type Currency string

// NewCurrency canonicalizes a code to uppercase.
func NewCurrency(code string) Currency {
	return Currency(strings.ToUpper(strings.TrimSpace(code)))
}

// Amount is a value of a single currency. The zero Amount has an empty
// currency and value zero; arithmetic against it fails with
// ErrCurrencyMismatch unless currencies match.
type Amount struct {
	currency Currency
	value    decimal.Decimal
}

// NewAmount creates an amount of the given currency.
func NewAmount(c Currency, v decimal.Decimal) Amount {
	return Amount{currency: c, value: v}
}

// Currency returns the amount's currency.
func (a Amount) Currency() Currency { return a.currency }

// Decimal returns the numeric value.
func (a Amount) Decimal() decimal.Decimal { return a.value }

// Add returns a+b, failing on currency mismatch.
func (a Amount) Add(b Amount) (Amount, error) {
	if a.currency != b.currency {
		return Amount{}, fmt.Errorf("add %s to %s: %w", b.currency, a.currency, ErrCurrencyMismatch)
	}
	return Amount{currency: a.currency, value: a.value.Add(b.value)}, nil
}

// Sub returns a-b, failing on currency mismatch.
func (a Amount) Sub(b Amount) (Amount, error) {
	if a.currency != b.currency {
		return Amount{}, fmt.Errorf("subtract %s from %s: %w", b.currency, a.currency, ErrCurrencyMismatch)
	}
	return Amount{currency: a.currency, value: a.value.Sub(b.value)}, nil
}

// Mul scales the amount by a dimensionless factor.
func (a Amount) Mul(f decimal.Decimal) Amount {
	return Amount{currency: a.currency, value: a.value.Mul(f)}
}

// IsZero reports whether the value is zero.
func (a Amount) IsZero() bool { return a.value.IsZero() }

// IsNegative reports whether the value is negative.
func (a Amount) IsNegative() bool { return a.value.IsNegative() }

// String formats as "1.5 BTC".
func (a Amount) String() string {
	return a.value.String() + " " + string(a.currency)
}
