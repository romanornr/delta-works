package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/romanornr/delta-works/internal/contracts"
	"github.com/romanornr/delta-works/internal/models"
	"github.com/shopspring/decimal"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
)

const (
	holdingsUpdateInterval = 10 * time.Minute
)

// HoldingsService manages account holdings with dependency injection
type holdingsService struct {
	engine   contracts.EngineService
	repo     contracts.RepositoryService
	logger   contracts.Logger
	holdings map[string]map[asset.Item]models.AccountHoldings
	mu       sync.RWMutex

	// Lifecycle management
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

func NewHoldingsService(engine contracts.EngineService, repo contracts.RepositoryService, logger contracts.Logger) contracts.HoldingsService {
	return &holdingsService{
		engine:   engine,
		repo:     repo,
		logger:   logger,
		holdings: make(map[string]map[asset.Item]models.AccountHoldings),
		done:     make(chan struct{}),
	}
}

func (h *holdingsService) UpdateHoldings(ctx context.Context, exchangeName string, accountType asset.Item) error {
	exch, err := h.engine.GetExchangeByName(exchangeName)
	if err != nil {
		return fmt.Errorf("exchange %s not found: %w", exchangeName, err)
	}

	accountInfo, err := exch.UpdateAccountInfo(ctx, accountType)
	if err != nil {
		return fmt.Errorf("failed to update account info for %s %s: %w", exchangeName, accountType, err)
	}

	holdings := models.AccountHoldings{
		ExchangeName: exchangeName,
		AccountType:  accountType,
		Balances:     make(map[currency.Code]models.AssetBalance),
		LastUpdated:  time.Now(),
	}

	var totalUSDValue decimal.Decimal

	// process balance
	for _, a := range accountInfo.Accounts {
		for _, balance := range a.Currencies {
			amount := decimal.NewFromFloat(balance.Total)
			var usdValue decimal.Decimal

			if balance.Currency.IsStableCurrency() {
				usdValue = amount
			} else {
				usdValue = amount.Mul(decimal.NewFromFloat(5000)) // TODO fetch actual rates for more accuracy
			}

			holdings.Balances[balance.Currency] = models.AssetBalance{
				Currency:               balance.Currency,
				Total:                  decimal.NewFromFloat(balance.Total),
				Hold:                   decimal.NewFromFloat(balance.Hold),
				Free:                   decimal.NewFromFloat(balance.Free),
				AvailableWithoutBorrow: decimal.NewFromFloat(balance.AvailableWithoutBorrow),
				Borrowed:               decimal.NewFromFloat(balance.Borrowed),
				USDValue:               usdValue,
			}
			totalUSDValue = totalUSDValue.Add(usdValue)
		}
	}

	holdings.TotalUSDValue = totalUSDValue

	if err := h.saveHoldings(ctx, exchangeName, accountType, &holdings); err != nil {
		return fmt.Errorf("failed to save holdings: %v\n", err)
	}

	h.logger.Info().Msgf("Updated holdings for %s %s", exchangeName, accountType)
	return nil
}

func (h *holdingsService) GetHoldings(exchangeName string, accountType asset.Item) (*models.AccountHoldings, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	exchangeHoldings, exists := h.holdings[exchangeName] // reads from cache
	if !exists {
		return nil, fmt.Errorf("No holdings found for exchange %s", exchangeName)
	}

	holdings, exists := exchangeHoldings[accountType]
	if !exists {
		return nil, fmt.Errorf("No holdings found for exchange %s %s", exchangeName, accountType)
	}

	return &holdings, nil
}

func (h *holdingsService) StartContinuousUpdate(ctx context.Context) error {
	if h.ctx != nil {
		return fmt.Errorf("continuous update already started")
	}

	h.ctx, h.cancel = context.WithCancel(ctx)

	go func() {
		defer close(h.done)
		h.logger.Debug().Msg("Starting holdings update routine")

		updateTicker := time.NewTicker(holdingsUpdateInterval)

		for {
			select {
			case <-h.ctx.Done():
				h.logger.Info().Msg("Context cancelled, stopping holdings update routine")
				return
			case <-updateTicker.C:
				h.updateAllExchanges(h.ctx)
			}
		}
	}()
	return nil
}

func (h *holdingsService) updateAllExchanges(ctx context.Context) {
	exchanges := h.engine.GetExchanges()
	var wg sync.WaitGroup

	for _, exch := range exchanges {
		wg.Add(1)
		go func(exchangeName string) {
			defer wg.Done()
			if err := h.UpdateHoldings(ctx, exchangeName, asset.Spot); err != nil {
				h.logger.Error().Err(err).Msgf("Failed to update holdings for %s", exchangeName)
			} else {
				h.logger.Debug().Msgf("Updated holdings for %s", exchangeName)
			}
		}(exch.GetName())
	}

	wg.Wait()
	h.logger.Debug().Msg("Holdings updated for all exchanges")
}

// Stop stops the holdings service
func (h *holdingsService) Stop(ctx context.Context) error {
	if h.cancel != nil {
		h.cancel()

		select {
		case <-h.done:
			h.logger.Info().Msg("Holdings service stopped")
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for holdings service to stop")
		}
	}
	// If never started, just return nil
	return nil
}

// saveHoldings saves the account holdings for a specific exchange and account type.
// if the mapping for the exchange doesn't exist, create one and add the account type with the holdings.
// update the cache and insert the holdings into the database
func (h *holdingsService) saveHoldings(ctx context.Context, exchangeName string, accountType asset.Item, holdings *models.AccountHoldings) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// update cache
	if _, exists := h.holdings[exchangeName]; !exists {
		h.holdings[exchangeName] = make(map[asset.Item]models.AccountHoldings)
	}
	h.holdings[exchangeName][accountType] = *holdings // update cache

	if err := h.repo.InsertHoldings(ctx, *holdings); err != nil {
		return fmt.Errorf("failed to insert holdings into QuestDB: %v\n", err)
	}
	return nil
}
