// Package money defines canonical currency codes. Monetary quantities use
// decimal values, and their owning domain records carry currencies separately.
package money

import "strings"

// Currency is a canonical uppercase asset code, e.g. "BTC", "USDT".
type Currency string

// NewCurrency canonicalizes a code to uppercase.
func NewCurrency(code string) Currency {
	return Currency(strings.ToUpper(strings.TrimSpace(code)))
}
