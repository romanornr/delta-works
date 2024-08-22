package core

import (
	"context"
	"fmt"
	"github.com/shopspring/decimal"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/engine"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"sync"
	"time"
)

type AssetBalance struct {
	Currency               currency.Code
	Total                  decimal.Decimal
	Hold                   decimal.Decimal
	Free                   decimal.Decimal
	AvailableWithoutBorrow decimal.Decimal
	Borrowed               decimal.Decimal
}

type AccountHoldings struct {
	ExchangeName string
	AccountType  asset.Item
	Balances     map[currency.Code]AssetBalance
	LastUpdated  time.Time
}

// HoldingsManager manages account holdings for multiple exchanges and account types
type HoldingsManager struct {
	instance *Instance
	holdings map[string]map[asset.Item]AccountHoldings
	mu       sync.RWMutex
}

func NewHoldingsManager(i *Instance) *HoldingsManager {
	return &HoldingsManager{
		instance: i,
		holdings: make(map[string]map[asset.Item]AccountHoldings),
	}
}

func (h *HoldingsManager) UpdateHoldings(ctx context.Context, exchangeName string, accountType asset.Item) error {
	if engine.Bot == nil {
		return fmt.Errorf("engine instance not set")
	}

	exchange, err := engine.Bot.ExchangeManager.GetExchangeByName(exchangeName)
	if err != nil {
		return fmt.Errorf("exchange %s not found", exchangeName)
	}

	acccountInfo, err := exchange.FetchAccountInfo(ctx, accountType)
	if err != nil {
		return fmt.Errorf("failed to fetch account info for %s %s: %v", exchangeName, accountType, err)
	}

	holdings := &AccountHoldings{
		ExchangeName: exchangeName,
		AccountType:  accountType,
		Balances:     make(map[currency.Code]AssetBalance),
		LastUpdated:  time.Now(),
	}

	for _, account := range acccountInfo.Accounts {
		for _, balance := range account.Currencies {
			holdings.Balances[balance.Currency] = AssetBalance{
				Currency:               balance.Currency,
				Total:                  decimal.NewFromFloat(balance.Total),
				Hold:                   decimal.NewFromFloat(balance.Hold),
				Free:                   decimal.NewFromFloat(balance.Free),
				AvailableWithoutBorrow: decimal.NewFromFloat(balance.AvailableWithoutBorrow),
				Borrowed:               decimal.NewFromFloat(balance.Borrowed),
			}
		}
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.holdings[exchangeName]; !exists {
		h.holdings[exchangeName] = make(map[asset.Item]AccountHoldings)
	}

	h.holdings[exchangeName][accountType] = *holdings

	fmt.Printf("Updated holdings for %s %s\n", exchangeName, accountType)
	fmt.Printf("Holdings: %+v\n", holdings)

	return nil
}
