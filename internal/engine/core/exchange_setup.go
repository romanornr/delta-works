package core

import (
	"context"
	"fmt"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/engine"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	gctlog "github.com/thrasher-corp/gocryptotrader/log"
)

func SetupExchangePairs(ctx context.Context) error {

	exchanges := engine.Bot.GetExchanges()
	if len(exchanges) == 0 {
		return fmt.Errorf("no exchanges found")
	}

	portfolioCurrencies, err := GetPortfolioCurrencies(ctx)
	if err != nil {
		return fmt.Errorf("failed to get portfolio currencies: %w", err)
	}

	for _, exch := range exchanges {
		err = exch.UpdateTradablePairs(ctx, false)
		if err != nil {
			gctlog.Warnf(gctlog.Global, "failed to update tradable pairs for %s: %v", exch.GetName(), err)
			continue
		}

		var pairsToEnable currency.Pairs
		quoteCurrencies := []currency.Code{currency.USDT, currency.USDC}

		for _, baseCurrency := range portfolioCurrencies {
			for _, quoteCurrency := range quoteCurrencies {
				pair := currency.NewPair(baseCurrency, quoteCurrency)
				//if err := exch.GetBase().SupportsPair(pair, false, asset.Spot); err == nil {
				//	pairsToEnable = append(pairsToEnable, pair)
				//}
				if exch.GetBase().CurrencyPairs.Pairs[asset.Spot].Available.Contains(pair, true) {
					pairsToEnable = append(pairsToEnable, pair)
				}
			}
		}

		if len(pairsToEnable) == 0 {
			gctlog.Warnf(gctlog.Global, "no tradable pairs found for %s", exch.GetName())
			continue
		}

		if errStorePair := exch.GetBase().CurrencyPairs.StorePairs(asset.Spot, pairsToEnable, true); errStorePair != nil {
			fmt.Printf("failed to store pairs for %s: %v\n", exch.GetName(), errStorePair)
			gctlog.Warnf(gctlog.Global, "failed to store pairs for %s: %v", exch.GetName(), errStorePair)
			continue
			//return fmt.Errorf("failed to store pairs for %s: %w", exch.GetName(), err)
		}
		gctlog.Infof(gctlog.Global, "%s tradable pairs enabled: %s", exch.GetName(), pairsToEnable)
	}

	return nil
}
