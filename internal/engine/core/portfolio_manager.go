package core

import (
	"context"
	"fmt"

	"github.com/romanornr/delta-works/internal/logger"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/engine"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
)

// GetPortfolioCurrencies retrieves the list of unique currencies present in the portfolio.
// It iterates over each exchange in the engine and fetches the account information for spot trading.
// It then checks the balances of each currency in the account and adds the non-zero balances to the uniqueCurrencies set.
// Finally, it converts the uniqueCurrencies set to a slice and returns it along with a nil error.
func GetPortfolioCurrencies(ctx context.Context) ([]currency.Code, error) {
	// Check if context is cancelled or has a deadline
	if ctx.Err() != nil {
		return nil, fmt.Errorf("context cancelled or deadline exceeded: %v", ctx.Err())
	}

	uniqueCurrencies := make(map[currency.Code]struct{})

	// Iterate over each exchange to fetch account information and collect unique currencies
	for _, exch := range engine.Bot.GetExchanges() {
		accountInfo, err := exch.UpdateAccountInfo(ctx, asset.Spot)
		if err != nil {
			return nil, fmt.Errorf("failed to update account info for %s: %w", exch.GetName(), err)
		}

		// DEBUG: Log exchange being processed
		logger.Debug().Msgf("Processing portfolio currencies for exchange: %s", exch.GetName())

		// collect currencies with non-zero balances
		for _, account := range accountInfo.Accounts {
			for _, balance := range account.Currencies {
				// DEBUG: Log all balances being checked
				logger.Debug().Msgf("Checking balance for %s: Total=%f, Free=%f, Hold=%f",
					balance.Currency.String(), balance.Total, balance.Free, balance.Hold)

				if balance.Total > 0 {
					uniqueCurrencies[balance.Currency] = struct{}{}
					logger.Debug().Msgf("Added %s to portfolio currencies (Total: %f)",
						balance.Currency.String(), balance.Total)
				}
			}
		}
	}

	// Convert the uniqueCurrencies map to a slice
	currencies := make([]currency.Code, 0, len(uniqueCurrencies))
	for c := range uniqueCurrencies {
		currencies = append(currencies, c)
	}

	// DEBUG: Log final portfolio currencies
	logger.Info().Msgf("Final portfolio currencies detected: %v", currencies)

	return currencies, nil
}
