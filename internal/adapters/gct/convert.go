package gct

import (
	"fmt"
	"strings"

	"github.com/romanornr/delta-works/internal/domain/market"
	"github.com/romanornr/delta-works/internal/domain/portfolio"
	"github.com/romanornr/delta-works/internal/errs"
	"github.com/shopspring/decimal"
	"github.com/thrasher-corp/gocryptotrader/exchange/accounts"
	"github.com/thrasher-corp/gocryptotrader/exchanges/ticker"
)

// ToTicker converts a GCT ticker to a market.Ticker
func ToTicker(exchangeName string, t *ticker.Price) (*market.Ticker, error) {
	if t == nil {
		return nil, fmt.Errorf("failed to convert gct ticker: ticker is nil: %w", errs.ErrTickerNotFound)
	}

	exchange := strings.ToLower(exchangeName)
	if exchange == "" {
		return nil, fmt.Errorf("failed to convert gct ticker: exchange name is empty: %w", errs.ErrExchangeNotFound)
	}

	base := portfolio.NormalizeAsset(t.Pair.Base.String())
	quote := portfolio.NormalizeAsset(t.Pair.Quote.String())
	if base == "" || quote == "" {
		return nil, fmt.Errorf("failed to convert gct ticker for exchange %s: base or quote is empty: %w", exchange, errs.ErrInvalidPair)
	}

	result := market.NewTicker(exchange, base, quote, decimal.NewFromFloat(t.Last), t.LastUpdated)

	bid := decimal.NewFromFloat(t.Bid)
	ask := decimal.NewFromFloat(t.Ask)

	if !bid.IsZero() && !ask.IsZero() {
		result.Top = &market.TopOfBook{
			Bid: bid,
			Ask: ask,
		}
	}

	return result, nil
}

// ToHoldings converts GCT sub-account balances into holdings.
func ToHoldings(subAccounts accounts.SubAccounts) ([]portfolio.Holding, error) {
	holdings := make([]portfolio.Holding, 0)

	for _, subAccount := range subAccounts {
		if subAccount == nil {
			continue
		}

		for assetCode, balance := range subAccount.Balances {
			asset := portfolio.NormalizeAsset(assetCode.String())
			if asset == "" {
				return nil, fmt.Errorf("failed to convert gct balance for sub-account %s: asset is empty: %w", subAccount.ID, errs.ErrInvalidCurrency)
			}

			total := decimal.NewFromFloat(balance.Total)
			available := decimal.NewFromFloat(balance.Free)
			locked := decimal.NewFromFloat(balance.Hold)
			availableWithoutBorrow := decimal.NewFromFloat(balance.AvailableWithoutBorrow)
			borrow := decimal.NewFromFloat(balance.Borrowed)

			holding := portfolio.NewHolding(
				asset,
				total,
				available,
				locked,
				availableWithoutBorrow,
				borrow,
				decimal.Zero,
			)

			holdings = append(holdings, holding)
		}
	}

	return holdings, nil
}
