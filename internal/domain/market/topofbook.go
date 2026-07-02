package market

import "github.com/shopspring/decimal"

// TopOfBook contains best bid-ask data
// Adapters should only set Top when bid and ask meaningful values
// If top-of-book data is missing or unusable, Top should be nil
type TopOfBook struct {
	Bid decimal.Decimal `json:"bid"`
	Ask decimal.Decimal `json:"ask"`
}

// Spread returns the bid-ask spread
func (t *TopOfBook) Spread() decimal.Decimal {
	return t.Ask.Sub(t.Bid)
}

// MidPrice returns the mid-price between the bid and ask
func (t *TopOfBook) MidPrice() decimal.Decimal {
	return t.Bid.Add(t.Ask).Div(decimal.NewFromInt(2))
}

// SpreadPercent returns the spread as a percentage of the mid-price
func (t *TopOfBook) SpreadPercent() decimal.Decimal {
	mid := t.MidPrice()
	if mid.IsZero() {
		return decimal.Zero
	}
	return t.Spread().Div(mid).Mul(decimal.NewFromInt(100))
}
