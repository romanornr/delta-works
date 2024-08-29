package core

import (
	"context"
	"fmt"
	"github.com/romanornr/delta-works/internal/models"
	"github.com/romanornr/delta-works/internal/repository"
	"github.com/shopspring/decimal"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/engine"
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

	exchange, err := engine.Bot.ExchangeManager.GetExchangeByName(exchangeName)
	if err != nil {
		return fmt.Errorf("exchange %s not found", exchangeName)
	}

	acccountInfo, err := exchange.FetchAccountInfo(ctx, accountType)
	if err != nil {
		return fmt.Errorf("failed to fetch account info for %s %s: %v", exchangeName, accountType, err)
	}

	holdings := &models.AccountHoldings{
		ExchangeName: exchangeName,
		AccountType:  accountType,
		Balances:     make(map[currency.Code]models.AssetBalance),
		LastUpdated:  time.Now(),
	}

	for _, account := range acccountInfo.Accounts {
		for _, balance := range account.Currencies {
			holdings.Balances[balance.Currency] = models.AssetBalance{
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
