package core

import (
	"context"
	"fmt"
	"github.com/thrasher-corp/gocryptotrader/config"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/engine"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	gctlog "github.com/thrasher-corp/gocryptotrader/log"
	"sync"
	"time"
)

var (
	instance *Instance
	once     sync.Once
)

type Instance struct {
	Settings *engine.Settings
	FlagSet  map[string]bool
}

func GetInstance(ctx context.Context, settings *engine.Settings, flagset map[string]bool) (*Instance, error) {

	var err error
	once.Do(func() {
		instance = &Instance{
			Settings: settings,
			FlagSet:  flagset,
		}

		err = instance.Initialize(ctx)
		if err == nil {
			config.SetConfig(engine.Bot.Config)
		}
	})

	return instance, err
}

func (i *Instance) Initialize(ctx context.Context) error {

	errChan := make(chan error, 1)
	done := make(chan struct{})

	go func() {
		var err error
		engine.Bot, err = engine.NewFromSettings(i.Settings, i.FlagSet)
		if err != nil {
			errChan <- err
		} else {
			close(done)
		}
	}()

	select {
	case <-ctx.Done():
		return fmt.Errorf("engine initialization cancelled: %v", ctx.Err())
	case err := <-errChan:
		return fmt.Errorf("failed to create engine: %v", err)
	case <-done:
		if engine.Bot == nil {
			return fmt.Errorf("engine initialization failed: Bot is nil")
		}
		gctlog.Debugln(gctlog.Global, "Engine successfully initialized")
		return nil
	}
}

func (i *Instance) StartEngine(ctx context.Context) error {
	if engine.Bot == nil {
		return fmt.Errorf("engine not initialized")
	}

	errChan := make(chan error, 1)
	go func() {
		errChan <- engine.Bot.Start()
	}()

	select {
	case <-ctx.Done():
		return fmt.Errorf("engine start cancelled: %v", ctx.Err())
	case err := <-errChan:
		if err != nil {
			return fmt.Errorf("failed to start engine: %v", err)
		}
		gctlog.Infoln(gctlog.Global, "Engine successfully started")
		return nil
	}
}

func (i *Instance) StopEngine(ctx context.Context) error {
	if engine.Bot == nil {
		return fmt.Errorf("engine not initialized")
	}

	done := make(chan struct{})
	var err error

	go func() {
		engine.Bot.Stop()
		close(done)
	}()

	select {
	case <-ctx.Done():
		// Context was cancelled, but we'll still wait a bit for the engine to stop
		select {
		case <-done:
			// Engine stopped before our additional timeout
		case <-time.After(5 * time.Second):
			err = fmt.Errorf("engine stop canceled: %w", ctx.Err())
		}
	case <-done:
		// Engine stopped normally
	}

	return err
}

// GetPortfolioCurrencies retrieves the list of unique currencies present in the portfolio.
// It iterates over each exchange in the engine and fetches the account information for spot trading.
// It then checks the balances of each currency in the account and adds the non-zero balances to the uniqueCurrencies set.
// Finally, it converts the uniqueCurrencies set to a slice and returns it along with a nil error.
func GetPortfolioCurrencies(ctx context.Context) ([]currency.Code, error) {
	uniqueCurrencies := make(map[currency.Code]struct{})

	for _, exch := range engine.Bot.GetExchanges() {
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

	currencies := make([]currency.Code, 0, len(uniqueCurrencies))
	for c := range uniqueCurrencies {
		currencies = append(currencies, c)
	}

	return currencies, nil
}

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
			return fmt.Errorf("failed to update tradable pairs for %s: %w", exch.GetName(), err)
		}

		var pairsToEnable currency.Pairs
		quoteCurrencies := []currency.Code{currency.USDT, currency.USDC}

		for _, baseCurrency := range portfolioCurrencies {
			for _, quoteCurrency := range quoteCurrencies {
				pair := currency.NewPair(baseCurrency, quoteCurrency)
				if err := exch.GetBase().SupportsPair(pair, false, asset.Spot); err == nil {
					pairsToEnable = append(pairsToEnable, pair)
				}
			}
		}

		if errStorePair := exch.GetBase().CurrencyPairs.StorePairs(asset.Spot, pairsToEnable, true); errStorePair != nil {
			return fmt.Errorf("failed to store pairs for %s: %w", exch.GetName(), err)
		}
		gctlog.Infof(gctlog.Global, "%s tradable pairs enabled: %s", exch.GetName(), pairsToEnable)
	}

	return nil
}
