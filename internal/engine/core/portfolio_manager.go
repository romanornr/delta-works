package core

import (
	"context"
	"fmt"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/engine"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
)

// GetPortfolioCurrencies retrieves the list of unique currencies present in the portfolio.
// It iterates over each exchange in the engine and fetches the account information for spot trading.
// It then checks the balances of each currency in the account and adds the non-zero balances to the uniqueCurrencies set.
// Finally, it converts the uniqueCurrencies set to a slice and returns it along with a nil error.
func GetPortfolioCurrencies(ctx context.Context) ([]currency.Code, error) {
	uniqueCurrencies := make(map[currency.Code]struct{})

	for _, exch := range engine.Bot.GetExchanges() {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("context cancelled: %v", ctx.Err())
		default:
			accountInfo, err := exch.FetchAccountInfo(ctx, asset.Spot)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch account info for %s: %w", exch.GetName(), err)
			}
			for _, account := range accountInfo.Accounts {
				for _, balance := range account.Currencies {
					if balance.Total > 0 {
						uniqueCurrencies[balance.Currency] = struct{}{}
					}
				}
			}
		}
	}

	currencies := make([]currency.Code, 0, len(uniqueCurrencies))
	for c := range uniqueCurrencies {
		currencies = append(currencies, c)
	}

	return currencies, nil
}
