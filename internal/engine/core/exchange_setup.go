package core

import (
	"context"
	"errors"
	"fmt"
	"github.com/romanornr/delta-works/internal/logger"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/engine"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
)

const (
	logNoExchanges      = "no exchanges found"
	logUpdatePairsError = "failed to update tradeable pairs for %s: %v"
	logNoTradeablePairs = "no tradeable pairs found for %s"
	logStorePairsError  = "failed to store pairs for %s: %v"
	logPairsEnabled     = "%s tradeable pairs enabled: %s"
)

// SetupExchangePairs initializes exchange pairs for trading based on the portfolio's available currencies and predefined quote currencies.
func SetupExchangePairs(ctx context.Context) error {

	exchanges := engine.Bot.GetExchanges()
	if len(exchanges) == 0 {
		return errors.New(logNoExchanges)
	}

	portfolioCurrencies, err := GetPortfolioCurrencies(ctx)
	if err != nil {
		return fmt.Errorf("failed to get portfolio currencies: %w", err)
	}

	for _, exch := range exchanges {
		err = exch.UpdateTradablePairs(ctx, false)
		if err != nil {
			logger.Warn().Msgf(logUpdatePairsError, exch.GetName(), err)
			continue
		}

		var enabledPairs currency.Pairs
		quoteCurrencies := []currency.Code{currency.USDT, currency.USDC}

		enabledPairs = getTradeablePairs(exch, portfolioCurrencies, quoteCurrencies)

		if len(enabledPairs) == 0 {
			logger.Warn().Msgf(logNoTradeablePairs, exch.GetName())
			continue
		}

		if errStorePair := exch.GetBase().CurrencyPairs.StorePairs(asset.Spot, enabledPairs, true); errStorePair != nil {
			logger.Warn().Msgf(logStorePairsError, exch.GetName(), errStorePair)
			continue
		}
		logger.Info().Msgf(logPairsEnabled, exch.GetName(), enabledPairs)
	}
	return nil
}

// getTradablePairs generates a list of tradeable currency pairs based on portfolio and quote currencies available on an exchange.
func getTradeablePairs(exch exchange.IBotExchange, portfolioCurrencies []currency.Code, quoteCurrencies []currency.Code) currency.Pairs {
	var pairsToEnable currency.Pairs
	for _, baseCurrency := range portfolioCurrencies {
		for _, quoteCurrency := range quoteCurrencies {
			pair := currency.NewPair(baseCurrency, quoteCurrency)
			if exch.GetBase().CurrencyPairs.Pairs[asset.Spot].Available.Contains(pair, true) {
				pairsToEnable = append(pairsToEnable, pair)
			}
		}
	}
	return pairsToEnable
}
