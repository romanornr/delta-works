package services_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/romanornr/delta-works/internal/contracts"
	"github.com/romanornr/delta-works/internal/models"
	"github.com/romanornr/delta-works/internal/services"
	"github.com/thrasher-corp/gocryptotrader/currency"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/account"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/ticker"
)

var errExchangeNotFound = errors.New("exchange not found")

// Mock implementations

// mockLogger implements contracts.Logger
type mockLogger struct{}

func (m *mockLogger) Info() contracts.LogEvent {
	return &mockLogEvent{}
}

func (m *mockLogger) Debug() contracts.LogEvent {
	return &mockLogEvent{}
}

func (m *mockLogger) Warn() contracts.LogEvent {
	return &mockLogEvent{}
}

func (m *mockLogger) Error() contracts.LogEvent {
	return &mockLogEvent{}
}

// mockLogEvent implements contracts.LogEvent
type mockLogEvent struct{}

func (m *mockLogEvent) Msg(msg string) {}

func (m *mockLogEvent) Msgf(format string, a ...any) {}

func (m *mockLogEvent) Err(err error) contracts.LogEvent {
	return m
}

func (m *mockLogEvent) Str(key, value string) contracts.LogEvent {
	return m
}

func (m *mockLogEvent) Int(key string, value int) contracts.LogEvent {
	return m
}

type mockEngineService struct {
	logger    contracts.Logger
	running   bool
	exchanges []contracts.ExchangeService
	mu        sync.RWMutex
}

func (m *mockEngineService) GetExchanges() []contracts.ExchangeService {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.exchanges
}

func (m *mockEngineService) GetExchangeByName(name string) (contracts.ExchangeService, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, exch := range m.exchanges {
		if exch.GetName() == name {
			return exch, nil
		}
	}

	return nil, errExchangeNotFound
}

func (m *mockEngineService) Start(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.running = true
	return nil
}

func (m *mockEngineService) Stop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.running = false
	return nil
}

func (m *mockEngineService) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.running
}

// mockExchangeService implements contracts.ExchangeService
type mockExchangeService struct {
	name string
}

func (m *mockExchangeService) GetName() string {
	return m.name
}

func (m *mockExchangeService) UpdateAccountInfo(ctx context.Context, assetType asset.Item) (account.Holdings, error) {
	holdings := account.Holdings{
		Exchange: m.name,
		Accounts: []account.SubAccount{
			{
				ID: "mock-account",
				Currencies: []account.Balance{
					{
						Currency:               currency.BTC,
						Total:                  1.5,
						Hold:                   0.5,
						Free:                   1.0,
						AvailableWithoutBorrow: 1.0,
						Borrowed:               0,
					},
					{
						Currency:               currency.USD,
						Total:                  10000,
						Hold:                   0,
						Free:                   10000,
						AvailableWithoutBorrow: 10000,
						Borrowed:               0,
					},
				},
			},
		},
	}
	return holdings, nil
}

func (m *mockExchangeService) UpdateTicker(ctx context.Context, pair currency.Pair, assetType asset.Item) (*ticker.Price, error) {
	return &ticker.Price{
		ExchangeName: m.name,
		Pair:         pair,
		Last:         50000,
		High:         51000,
		Low:          49000,
		Bid:          49900,
		Ask:          50100,
		Volume:       100,
		QuoteVolume:  5000000,
		PriceATH:     69000,
		Open:         49500,
		Close:        50000,
	}, nil
}

func (m *mockExchangeService) GetWithdrawalsHistory(ctx context.Context, currency currency.Code, assetType asset.Item) ([]exchange.WithdrawalHistory, error) {
	// return empty withdrawal history for mock
	return []exchange.WithdrawalHistory{}, nil
}

// mockRepository implements contracts.RepositoryService
type mockRepositoryService struct {
	holdings    []models.AccountHoldings
	withdrawals []exchange.WithdrawalHistory
	mu          sync.Mutex
}

func newMockRepositoryService() *mockRepositoryService {
	return &mockRepositoryService{
		holdings:    make([]models.AccountHoldings, 0),
		withdrawals: make([]exchange.WithdrawalHistory, 0),
	}
}

func (m *mockRepositoryService) InsertHoldings(ctx context.Context, holdings models.AccountHoldings) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.holdings = append(m.holdings, holdings)
	return nil
}

func (m *mockRepositoryService) StoreWithdrawal(ctx context.Context, exchange string, withdrawals []exchange.WithdrawalHistory) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.withdrawals = append(m.withdrawals, withdrawals...)
	return nil
}

func (m *mockRepositoryService) Close(ctx context.Context) error {
	return nil
}

func (m *mockRepositoryService) GetStoredHoldings() []models.AccountHoldings {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.holdings
}

// Test Implementation

func TestServiceIntegration_FullWorkflow(t *testing.T) {
	mockLogger := &mockLogger{}
	mockRepository := newMockRepositoryService()
	mockEngine := &mockEngineService{
		logger: mockLogger,
		exchanges: []contracts.ExchangeService{
			&mockExchangeService{name: "bybit"},
		},
	}

	holdingsService := services.NewHoldingsService(mockEngine, mockRepository, mockLogger)
	withdrawalsService := services.NewWithdrawalService(mockEngine, mockRepository, mockLogger)

	ctx := context.Background()

	if err := mockEngine.Start(ctx); err != nil {
		t.Fatalf("failed to start engine: %v", err)
	}

	if !mockEngine.IsRunning() {
		t.Error("Engine should be running")
	}

	if err := holdingsService.UpdateHoldings(ctx, "bybit", asset.Spot); err != nil {
		t.Fatalf("failed to update holdings: %v", err)
	}

	if err := holdingsService.UpdateHoldings(ctx, "bybit", asset.Spot); err != nil {
		t.Fatalf("failed to update holdings: %v", err)
	}

	withdrawals, err := withdrawalsService.FetchWithdrawalHistory(ctx, "bybit", currency.BTC, asset.Spot)
	if err != nil {
		t.Fatalf("failed to fetch withdrawal history: %v", err)
	}

	if len(withdrawals) != 0 {
		t.Errorf("expected 0 withdrawals, got %d", len(withdrawals))
	}

	// Verify data was sotred in repostitory
	storedHoldings := mockRepository.GetStoredHoldings()
	if len(storedHoldings) == 0 {
		t.Errorf("expected holdings to be stored, got %d", len(storedHoldings))
	}

	if err := mockEngine.Stop(ctx); err != nil {
		t.Fatalf("failed to stop engine: %v", err)
	}

	if mockEngine.IsRunning() {
		t.Errorf("Engine should not be running")
	}

	t.Log("Integration test completed successfully")

}

// // mockEngineService implements contracts.EngineService
// type mockEngineService struct {
// 	logger    contracts.Logger
// 	running   bool
// 	exchanges []contracts.ExchangeService
// 	mu        sync.RWMutex
// }

// func (m *mockEngineService) GetExchanges() []contracts.ExchangeService {
// 	m.mu.RLock()
// 	defer m.mu.RUnlock()
// 	return m.exchanges
// }

// func (m *mockEngineService) GetExchangeByName(name string) (contracts.ExchangeService, error) {
// 	m.mu.RLock()
// 	defer m.mu.RUnlock()

// 	for _, exch := range m.exchanges {
// 		if exch.GetName() == name {
// 			return exch, nil
// 		}
// 	}
// 	return nil, errExchangeNotFound
// }

// func (m *mockEngineService) Start(ctx context.Context) error {
// 	m.mu.Lock()
// 	defer m.mu.Unlock()
// 	m.running = true
// 	return nil
// }

// func (m *mockEngineService) Stop(ctx context.Context) error {
// 	m.mu.Lock()
// 	defer m.mu.Unlock()
// 	m.running = false
// 	return nil
// }

// func (m *mockEngineService) IsRunning() bool {
// 	m.mu.RLock()
// 	defer m.mu.RUnlock()
// 	return m.running
// }

// // mockExchangeService implements contracts.ExchangeService
// type mockExchangeService struct {
// 	name string
// }

// func (m *mockExchangeService) GetName() string {
// 	return m.name
// }

// func (m *mockExchangeService) UpdateAccountInfo(ctx context.Context, assetType asset.Item) (account.Holdings, error) {
// 	// Return mock account holdings
// 	holdings := account.Holdings{
// 		Exchange: m.name,
// 		Accounts: []account.SubAccount{
// 			{
// 				ID: "mock-account",
// 				Currencies: []account.Balance{
// 					{
// 						Currency:               currency.BTC,
// 						Total:                  1.5,
// 						Hold:                   0.5,
// 						Free:                   1.0,
// 						AvailableWithoutBorrow: 1.0,
// 						Borrowed:               0,
// 					},
// 					{
// 						Currency:               currency.USD,
// 						Total:                  10000,
// 						Hold:                   0,
// 						Free:                   10000,
// 						AvailableWithoutBorrow: 10000,
// 						Borrowed:               0,
// 					},
// 				},
// 			},
// 		},
// 	}
// 	return holdings, nil
// }

// func (m *mockExchangeService) UpdateTicker(ctx context.Context, pair currency.Pair, assetType asset.Item) (*ticker.Price, error) {
// 	return &ticker.Price{
// 		Pair:         pair,
// 		ExchangeName: m.name,
// 		Last:         50000,
// 		High:         51000,
// 		Low:          49000,
// 		Bid:          49900,
// 		Ask:          50100,
// 		Volume:       100,
// 		QuoteVolume:  5000000,
// 		PriceATH:     69000,
// 		Open:         49500,
// 		Close:        50000,
// 	}, nil
// }

// func (m *mockExchangeService) GetWithdrawalsHistory(ctx context.Context, currency currency.Code, assetType asset.Item) ([]exchange.WithdrawalHistory, error) {
// 	// Return empty withdrawal history for mock
// 	return []exchange.WithdrawalHistory{}, nil
// }

// // mockRepositoryService implements contracts.RepositoryService
// type mockRepositoryService struct {
// 	holdings    []models.AccountHoldings
// 	withdrawals []exchange.WithdrawalHistory
// 	mu          sync.Mutex
// }

// func newMockRepositoryService() *mockRepositoryService {
// 	return &mockRepositoryService{
// 		holdings:    make([]models.AccountHoldings, 0),
// 		withdrawals: make([]exchange.WithdrawalHistory, 0),
// 	}
// }

// func (m *mockRepositoryService) InsertHoldings(ctx context.Context, holdings models.AccountHoldings) error {
// 	m.mu.Lock()
// 	defer m.mu.Unlock()
// 	m.holdings = append(m.holdings, holdings)
// 	return nil
// }

// func (m *mockRepositoryService) StoreWithdrawal(ctx context.Context, exchange string, withdrawals []exchange.WithdrawalHistory) error {
// 	m.mu.Lock()
// 	defer m.mu.Unlock()
// 	m.withdrawals = append(m.withdrawals, withdrawals...)
// 	return nil
// }

// func (m *mockRepositoryService) Close(ctx context.Context) error {
// 	return nil
// }

// func (m *mockRepositoryService) GetStoredHoldings() []models.AccountHoldings {
// 	m.mu.Lock()
// 	defer m.mu.Unlock()
// 	return m.holdings
// }

// // Test implementation

// func TestServiceIntegration_FullWorkflow(t *testing.T) {
// 	// Create mock dependencies directly without using the container
// 	mockLogger := &mockLogger{}
// 	mockRepo := newMockRepositoryService()
// 	mockEngine := &mockEngineService{
// 		logger: mockLogger,
// 		exchanges: []contracts.ExchangeService{
// 			&mockExchangeService{name: "binance"},
// 		},
// 	}

// 	// Create services with mock dependencies
// 	holdingsService := services.NewHoldingsService(mockEngine, mockRepo, mockLogger)
// 	withdrawalService := services.NewWithdrawalService(mockEngine, mockRepo, mockLogger)

// 	ctx := context.Background()

// 	// Test engine service
// 	if err := mockEngine.Start(ctx); err != nil {
// 		t.Fatalf("Failed to start engine: %v", err)
// 	}

// 	if !mockEngine.IsRunning() {
// 		t.Error("Engine should be running")
// 	}

// 	// Test holdings service
// 	if err := holdingsService.UpdateHoldings(ctx, "binance", asset.Spot); err != nil {
// 		t.Fatalf("Failed to update holdings: %v", err)
// 	}

// 	// Test withdrawal service
// 	withdrawals, err := withdrawalService.FetchWithdrawalHistory(ctx, "binance", currency.BTC, asset.Spot)
// 	if err != nil {
// 		t.Fatalf("Failed to fetch withdrawal history: %v", err)
// 	}

// 	if len(withdrawals) != 0 {
// 		t.Errorf("Expected 0 withdrawals (mock returns empty), got %d", len(withdrawals))
// 	}

// 	// Verify data was stored in repository
// 	storedHoldings := mockRepo.GetStoredHoldings()

// 	if len(storedHoldings) == 0 {
// 		t.Error("Expected holdings to be stored in repository")
// 	}

// 	// Stop engine
// 	if err := mockEngine.Stop(ctx); err != nil {
// 		t.Fatalf("Failed to stop engine: %v", err)
// 	}

// 	if mockEngine.IsRunning() {
// 		t.Error("Engine should not be running")
// 	}

// 	t.Log("Integration test completed successfully")
// }

// func TestServiceIntegration_WithContainer(t *testing.T) {
// 	// Create service container with mock services
// 	logger := container.NewDefaultLogger()
// 	serviceContainer := container.NewServiceContainer(logger)

// 	// Register mock services for integration testing
// 	registerMockServices(serviceContainer)

// 	// Create application scope
// 	scopeID := "integration-test"
// 	serviceContainer.CreateScope(scopeID)
// 	defer serviceContainer.DisposeScope(scopeID)

// 	ctx := context.Background()

// 	// First, ensure all shared services are created to avoid deadlocks
// 	loggerType := reflect.TypeOf((*contracts.Logger)(nil)).Elem()
// 	engineType := reflect.TypeOf((*contracts.EngineService)(nil)).Elem()
// 	repoType := reflect.TypeOf((*contracts.RepositoryService)(nil)).Elem()
// 	holdingsType := reflect.TypeOf((*contracts.HoldingsService)(nil)).Elem()
// 	withdrawalType := reflect.TypeOf((*contracts.WithdrawalService)(nil)).Elem()

// 	// Get logger first (no dependencies)
// 	_, err := serviceContainer.Get(loggerType)
// 	if err != nil {
// 		t.Fatalf("Failed to get logger service: %v", err)
// 	}

// 	// Get repository service (depends on logger)
// 	repoService, err := serviceContainer.Get(repoType)
// 	if err != nil {
// 		t.Fatalf("Failed to get repository service: %v", err)
// 	}

// 	// Get engine service (depends on logger)
// 	engineService, err := serviceContainer.Get(engineType)
// 	if err != nil {
// 		t.Fatalf("Failed to get engine service: %v", err)
// 	}

// 	// Get holdings service (scoped, depends on engine, repo, logger)
// 	holdingsService, err := serviceContainer.GetScoped(holdingsType, scopeID)
// 	if err != nil {
// 		t.Fatalf("Failed to get holdings service: %v", err)
// 	}

// 	// Get withdrawal service (depends on engine, repo, logger)
// 	withdrawalService, err := serviceContainer.Get(withdrawalType)
// 	if err != nil {
// 		t.Fatalf("Failed to get withdrawal service: %v", err)
// 	}

// 	// Test engine service
// 	if err := engineService.(contracts.EngineService).Start(ctx); err != nil {
// 		t.Fatalf("Failed to start engine: %v", err)
// 	}

// 	if !engineService.(contracts.EngineService).IsRunning() {
// 		t.Error("Engine should be running")
// 	}

// 	// Test holdings service
// 	if err := holdingsService.(contracts.HoldingsService).UpdateHoldings(ctx, "binance", asset.Spot); err != nil {
// 		t.Fatalf("Failed to update holdings: %v", err)
// 	}

// 	// Test withdrawal service
// 	withdrawals, err := withdrawalService.(contracts.WithdrawalService).FetchWithdrawalHistory(ctx, "binance", currency.BTC, asset.Spot)
// 	if err != nil {
// 		t.Fatalf("Failed to fetch withdrawal history: %v", err)
// 	}

// 	if len(withdrawals) != 0 {
// 		t.Errorf("Expected 0 withdrawals (mock returns empty), got %d", len(withdrawals))
// 	}

// 	// Verify data was stored in repository
// 	mockRepo := repoService.(*mockRepositoryService)
// 	storedHoldings := mockRepo.GetStoredHoldings()

// 	if len(storedHoldings) == 0 {
// 		t.Error("Expected holdings to be stored in repository")
// 	}

// 	// Stop engine
// 	if err := engineService.(contracts.EngineService).Stop(ctx); err != nil {
// 		t.Fatalf("Failed to stop engine: %v", err)
// 	}

// 	if engineService.(contracts.EngineService).IsRunning() {
// 		t.Error("Engine should not be running")
// 	}

// 	t.Log("Integration test with container completed successfully")
// }

// func registerMockServices(serviceContainer *container.ServiceContainer) {
// 	// Create shared instances upfront to avoid deadlocks
// 	mockLoggerInstance := &mockLogger{}
// 	mockRepoInstance := newMockRepositoryService()
// 	mockEngineInstance := &mockEngineService{
// 		logger: mockLoggerInstance,
// 		exchanges: []contracts.ExchangeService{
// 			&mockExchangeService{name: "binance"},
// 		},
// 	}

// 	// Register mock logger
// 	loggerType := reflect.TypeOf((*contracts.Logger)(nil)).Elem()
// 	serviceContainer.RegisterSharedResource(loggerType, func(c *container.ServiceContainer) (interface{}, error) {
// 		return mockLoggerInstance, nil
// 	})

// 	// Register mock engine service
// 	engineType := reflect.TypeOf((*contracts.EngineService)(nil)).Elem()
// 	serviceContainer.RegisterSharedResource(engineType, func(c *container.ServiceContainer) (interface{}, error) {
// 		return mockEngineInstance, nil
// 	})

// 	// Register mock repository service
// 	repoType := reflect.TypeOf((*contracts.RepositoryService)(nil)).Elem()
// 	serviceContainer.RegisterSharedResource(repoType, func(c *container.ServiceContainer) (interface{}, error) {
// 		return mockRepoInstance, nil
// 	})

// 	// Register real holdings service - create it with dependencies upfront
// 	holdingsType := reflect.TypeOf((*contracts.HoldingsService)(nil)).Elem()
// 	serviceContainer.RegisterScopedResource(holdingsType, func(c *container.ServiceContainer) (interface{}, error) {
// 		// Use the pre-created instances instead of calling Get
// 		return services.NewHoldingsService(
// 			mockEngineInstance,
// 			mockRepoInstance,
// 			mockLoggerInstance,
// 		), nil
// 	})

// 	// Register real withdrawal service - create it with dependencies upfront
// 	withdrawalType := reflect.TypeOf((*contracts.WithdrawalService)(nil)).Elem()
// 	serviceContainer.RegisterAlwaysNew(withdrawalType, func(c *container.ServiceContainer) (interface{}, error) {
// 		// Use the pre-created instances instead of calling Get
// 		return services.NewWithdrawalService(
// 			mockEngineInstance,
// 			mockRepoInstance,
// 			mockLoggerInstance,
// 		), nil
// 	})
// }
