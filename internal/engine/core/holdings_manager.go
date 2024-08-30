package core

import (
	"context"
	"fmt"
	"github.com/romanornr/delta-works/internal/models"
	"github.com/romanornr/delta-works/internal/repository"
	"github.com/shopspring/decimal"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/engine"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
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

	for _, account := range accountInfo.Accounts {
		for _, balance := range account.Currencies {

			amount := decimal.NewFromFloat(balance.Total)
			price, err := h.getUSDValue(ctx, exch, balance.Currency, amount, accountType)
			if err != nil {
				fmt.Printf("Failed to get USD value for %s: %v\n", balance.Currency, err)
			}

			holdings.Balances[balance.Currency] = models.AssetBalance{
				Currency:               balance.Currency,
				Total:                  decimal.NewFromFloat(balance.Total),
				Hold:                   decimal.NewFromFloat(balance.Hold),
				Free:                   decimal.NewFromFloat(balance.Free),
				AvailableWithoutBorrow: decimal.NewFromFloat(balance.AvailableWithoutBorrow),
				Borrowed:               decimal.NewFromFloat(balance.Borrowed),
				USDValue:               price.Mul(amount),
			}

			fmt.Printf("USD value for %s: %s\n", balance.Currency, holdings.Balances[balance.Currency].USDValue.String())
			totalUSDValue = totalUSDValue.Add(holdings.Balances[balance.Currency].USDValue)
		}
	}

	holdings.TotalUSDValue = totalUSDValue

	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.holdings[exchangeName]; !exists {
		h.holdings[exchangeName] = make(map[asset.Item]models.AccountHoldings)
	}
	h.holdings[exchangeName][accountType] = *holdings

	// Insert holdings into QuestDB
	if err := h.repo.InsertHoldings(ctx, *holdings); err != nil {
		return fmt.Errorf("failed to insert holdings into QuestDB: %w", err)
	}

	fmt.Printf("Updated holdings for %s %s\n", exchangeName, accountType)

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
			fmt.Printf("Fetched ticker for %s with pair %s\n", c.String(), pairs)
			return decimal.NewFromFloat(ticker.Last), nil
			//return amount.Mul(decimal.NewFromFloat(ticker.Last)), nil
		}

		//// Try reverse pair if direct pair fails
		//reversePair := pair.Swap()
		//ticker, err = exchange.FetchTicker(ctx, reversePair, accountType)
		//if err == nil {
		//	return amount.Div(decimal.NewFromFloat(ticker.Last)), nil
		//}
	}
	fmt.Printf("Failed to fetch ticker for %s with pair %s\n", c.String(), pairs)
	return decimal.Zero, fmt.Errorf("failed to fetch ticker for %s with pair %s", c.String(), pairs)
}
