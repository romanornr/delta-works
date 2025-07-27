package services

import (
	"context"
	"fmt"
	"sync"

	"github.com/romanornr/delta-works/internal/contracts"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/engine"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/account"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/ticker"
)

// engineService wraps the gocryptotrader engine and provides a container for it
// This is used to abstract the engine operations from the rest of the application
// It also provides a thread-safe way to access the engine and implements the interface
type engineService struct {
	bot     *engine.Engine
	logger  contracts.Logger
	running bool
	mu      sync.RWMutex
}

// exchangeService wraps the gocryptotrader exchange and provides a container for it
// This is used to abstract the exchange operations from the rest of the application
type exchangeService struct {
	exchange exchange.IBotExchange
	logger   contracts.Logger
}

func NewEngineService(settings *engine.Settings, flagset map[string]bool, logger contracts.Logger) (contracts.EngineService, error) {
	bot, err := engine.NewFromSettings(settings, flagset)
	if err != nil {
		return nil, fmt.Errorf("failed to create engine: %w", err)
	}

	return &engineService{
		bot:    bot,
		logger: logger,
	}, nil
}

// GetExchangeByName returns the exchange by name
func (e *engineService) GetExchangeByName(name string) (contracts.ExchangeService, error) {
	exchange, err := e.bot.GetExchangeByName(name)
	if err != nil {
		return nil, fmt.Errorf("exchange %s not found: %w", name, err)
	}

	return &exchangeService{
		exchange: exchange,
		logger:   e.logger,
	}, nil
}

// Start starts the engine with state tracking
func (e *engineService) Start(ctx context.Context) error {
	errChan := make(chan error, 1)
	go func() {
		errChan <- e.bot.Start()
	}()

	select {
	case <-ctx.Done():
		return fmt.Errorf("engine start cancelled: %v", ctx.Err())
	case err := <-errChan:
		if err != nil {
			return fmt.Errorf("failed to start engine: %v", err)
		}
		e.mu.Lock()
		e.running = true
		e.mu.Unlock()
		e.logger.Info().Msg("Engine started successfully")
		return nil
	}
}

func (e *engineService) Stop(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.running {
		return nil // Already stopped
	}

	e.bot.Stop()

	e.running = false
	e.logger.Info().Msg("Engine stopped successfully")
	return nil
}

func (e *engineService) IsRunning() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Check if we have internal state tracking
	if e.running {
		return true
	}

	// Alternative: Check if any critical subsystems are running
	// This provides a fallback method using the engine's actual state
	subsystems := e.bot.GetSubsystemsStatus()

	// Consider engine running if any critical subsystems are active
	criticalSubsystems := []string{
		"CommunicationsManager",
		"ConnectionManager",
		"OrderManager",
		"PortfolioManager",
	}

	for _, subsystem := range criticalSubsystems {
		if status, exists := subsystems[subsystem]; exists && status {
			return true
		}
	}

	return false
}

// GetExchanges returns looped through exchanges and returns them as a slice of ExchangeService
// This is used to get all exchanges registered in the engine
func (e *engineService) GetExchanges() []contracts.ExchangeService {
	exchanges := e.bot.GetExchanges()
	result := make([]contracts.ExchangeService, len(exchanges))

	for i, exchange := range exchanges {
		result[i] = &exchangeService{
			exchange: exchange,
			logger:   e.logger,
		}
	}

	return result
}

// GetName returns the name of the exchange
func (es *exchangeService) GetName() string {
	return es.exchange.GetName()
}

// UpdateAccountInfo updates the account holdings for the exchange
func (es *exchangeService) UpdateAccountInfo(ctx context.Context, assetType asset.Item) (account.Holdings, error) {
	return es.exchange.UpdateAccountInfo(ctx, assetType)
}

// UpdateTicker updates the ticker for the exchange
func (es *exchangeService) UpdateTicker(ctx context.Context, pair currency.Pair, assetType asset.Item) (*ticker.Price, error) {
	return es.exchange.UpdateTicker(ctx, pair, assetType)
}

// GetWithdrawalsHistory returns the withdrawal history for the exchange
func (es *exchangeService) GetWithdrawalsHistory(ctx context.Context, currency currency.Code, assetType asset.Item) ([]exchange.WithdrawalHistory, error) {
	return es.exchange.GetWithdrawalsHistory(ctx, currency, assetType)
}
