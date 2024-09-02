package core

import (
	"context"
	"fmt"
	"github.com/romanornr/delta-works/internal/logger"
	"github.com/romanornr/delta-works/internal/models"
	"github.com/romanornr/delta-works/internal/repository"
	"github.com/shopspring/decimal"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/engine"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/account"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"sync"
	"time"
)

// HoldingsManager manages account holdings for multiple exchanges and account types
type HoldingsManager struct {
	instance *Instance
	repo     *repository.QuestDBRepository
	holdings map[string]map[asset.Item]models.AccountHoldings
	mu       sync.RWMutex
}

// NewHoldingsManager initializes a new HoldingsManager instance with the given Instance and QuestDB configuration.
// It creates a QuestDB repository using the provided config and returns an error if the creation process fails.
// The function returns the created HoldingsManager instance and a possible error.
func NewHoldingsManager(i *Instance, questDBConfig string) (*HoldingsManager, error) {
	repo, err := repository.NewQuestDBRepository(context.Background(), questDBConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create QuestDB repository: %w", err)
	}

	return &HoldingsManager{
		instance: i,
		repo:     repo,
		holdings: make(map[string]map[asset.Item]models.AccountHoldings),
	}, nil
}

func (h *HoldingsManager) UpdateHoldings(ctx context.Context, exchangeName string, accountType asset.Item) error {
	if engine.Bot == nil {
		return fmt.Errorf("engine instance not set")
	}

	exch, err := engine.Bot.ExchangeManager.GetExchangeByName(exchangeName)
	if err != nil {
		return fmt.Errorf("exchange %s not found", exchangeName)
	}

	accountInfo, err := exch.FetchAccountInfo(ctx, accountType)
	if err != nil {
		return fmt.Errorf("failed to fetch account info for %s %s: %v", exchangeName, accountType, err)
	}

	holdings := &models.AccountHoldings{
		ExchangeName: exchangeName,
		AccountType:  accountType,
		Balances:     make(map[currency.Code]models.AssetBalance),
		LastUpdated:  time.Now(),
	}

	var totalUSDValue decimal.Decimal
	var wg sync.WaitGroup
	balanceChan := make(chan models.AssetBalance, len(accountInfo.Accounts)*len(accountInfo.Accounts[0].Currencies)) // buffer size is the number of balances to process
	errChan := make(chan error, len(accountInfo.Accounts)*len(accountInfo.Accounts[0].Currencies))

	for _, a := range accountInfo.Accounts {
		for _, balance := range a.Currencies {
			wg.Add(1)
			go func(balance account.Balance) {
				defer wg.Done()

				amount := decimal.NewFromFloat(balance.Total)
				var usdValue decimal.Decimal

				if balance.Currency.IsStableCurrency() {
					usdValue = amount
				} else {
					price, err := h.getUSDValue(ctx, exch, balance.Currency, amount, accountType)
					if err != nil {
						errChan <- fmt.Errorf("failed to get USD value for %s: %w", balance.Currency, err)
						return
					}
					usdValue = amount.Mul(price)
				}

				balanceChan <- models.AssetBalance{
					Currency:               balance.Currency,
					Total:                  decimal.NewFromFloat(balance.Total),
					Hold:                   decimal.NewFromFloat(balance.Hold),
					Free:                   decimal.NewFromFloat(balance.Free),
					AvailableWithoutBorrow: decimal.NewFromFloat(balance.AvailableWithoutBorrow),
					Borrowed:               decimal.NewFromFloat(balance.Borrowed),
					USDValue:               usdValue,
				}
			}(balance)
		}
	}

	go func() {
		wg.Wait()
		close(balanceChan)
		close(errChan)
	}()

	for balance := range balanceChan {
		holdings.Balances[balance.Currency] = balance
		totalUSDValue = totalUSDValue.Add(balance.USDValue)
	}

	holdings.TotalUSDValue = totalUSDValue

	if err := h.saveHoldings(ctx, exchangeName, accountType, holdings); err != nil {
		return fmt.Errorf("failed to save holdings: %v", err)
	}

	logger.Info().Msgf("Updated holdings for %s %s", exchangeName, accountType)
	return nil
}

// saveHoldings saves the account holdings for a specific exchange and account type.
// It obtains a lock to safely access and update the holdings data, creates a new map entry if necessary,
// and updates the specified holdings with the new values. It then inserts the holdings into
// a QuestDB repository and prints a confirmation message. If any error occurs during the process,
// it returns an error.
func (h *HoldingsManager) saveHoldings(ctx context.Context, exchangeName string, accountType asset.Item, holdings *models.AccountHoldings) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, exists := h.holdings[exchangeName]; !exists {
		h.holdings[exchangeName] = make(map[asset.Item]models.AccountHoldings)
	}
	h.holdings[exchangeName][accountType] = *holdings
	if err := h.repo.InsertHoldings(ctx, *holdings); err != nil {
		return fmt.Errorf("failed to insert holdings into QuestDB: %v\n", err)
	}
	logger.Info().Msgf("Updated holdings for %s %s", exchangeName, accountType)
	return nil
}

func (h *HoldingsManager) getUSDValue(ctx context.Context, exchange exchange.IBotExchange, c currency.Code, amount decimal.Decimal, accountType asset.Item) (decimal.Decimal, error) {

	if c == currency.USD {
		return amount, nil
	}

	//if c.IsFiatCurrency() {
	//	usdValue, err := currency.ConvertFiat(amount.InexactFloat64(), c, currency.USD)
	//	if err != nil {
	//		return decimal.Zero, fmt.Errorf("failed to convert %s to USD: %w", c, err)
	//	}
	//	return decimal.NewFromFloat(usdValue), nil
	//}

	if c.IsStableCurrency() {
		// Assume 1:1 for stablecoins, TODO fetch actual rates for more accuracy
		return amount, nil
	}

	//	if c.IsCryptocurrency() {
	// create pairs to fetch ticker
	pairs := []currency.Pair{
		currency.NewPair(c, currency.USDT),
		currency.NewPair(c, currency.USDC),
	}

	for _, pair := range pairs {
		ticker, fetchErr := exchange.FetchTicker(ctx, pair, accountType)
		if fetchErr == nil {
			return decimal.NewFromFloat(ticker.Last), nil
		}

		//// Try reverse pair if direct pair fails
		//reversePair := pair.Swap()
		//ticker, err = exchange.FetchTicker(ctx, reversePair, accountType)
		//if err == nil {
		//	return amount.Div(decimal.NewFromFloat(ticker.Last)), nil
		//}
	}
	logger.Warn().Msgf("Failed to fetch ticker for %s with pair %s", c.String(), pairs)
	return decimal.Zero, fmt.Errorf("failed to fetch ticker for %s with pair %s", c.String(), pairs)
}
