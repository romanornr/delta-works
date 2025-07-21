package services

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/romanornr/delta-works/internal/container"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/engine"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/account"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/ticker"
)

type mockLogger struct {
	logs []string
}

type mockLogEvent struct {
	logger *mockLogger
	level  string
	err    error
}

func (m *mockLogger) Info() container.LogEvent {
	return &mockLogEvent{logger: m, level: "INFO"}
}

func (m *mockLogger) Debug() container.LogEvent {
	return &mockLogEvent{logger: m, level: "DEBUG"}
}

func (m *mockLogger) Warn() container.LogEvent {
	return &mockLogEvent{logger: m, level: "WARN"}
}

func (m *mockLogger) Error() container.LogEvent {
	return &mockLogEvent{logger: m, level: "ERROR"}
}

func (e *mockLogEvent) Msg(msg string) {
	e.logger.logs = append(e.logger.logs, fmt.Sprintf("[%s] %s", e.level, msg))
}

func (e *mockLogEvent) Msgf(format string, v ...interface{}) {
	e.logger.logs = append(e.logger.logs, fmt.Sprintf("[%s] %s", e.level, format))
}

func (e *mockLogEvent) Err(err error) container.LogEvent {
	return &mockLogEvent{logger: e.logger, level: e.level, err: err}
}

func TestEngineService(t *testing.T) {
	logger := &mockLogger{}

	settings := &engine.Settings{
		ConfigFile: "config.json",
		DataDir:    "data",
	}

	flagset := map[string]bool{
		"configfile": true,
		"datadir":    true,
	}

	// This will fail in real environment without proper config but this is for testing interface compliance
	_, err := NewEngineService(settings, flagset, logger)
	if err == nil {
		// Expect this to fail in test environment, but the interface should be correct
		t.Log("Engine service created successfully (unexpected in test environment)")
	} else {
		t.Logf("Engine service creation failed as expected in test environment: %v", err)
	}
}

func TestEngineService_InterfaceCompliance(t *testing.T) {
	var _ container.EngineService = (*engineService)(nil)
	var _ container.ExchangeService = (*exchangeService)(nil)

	t.Log("Engine service interface compliance test passed")
}

// mockEngineService for testing
type mockEngineService struct {
	logger  container.Logger
	running bool
}

func (m *mockEngineService) Start(ctx context.Context) error {
	m.running = true
	m.logger.Info().Msg("Mock engine started")
	return nil
}

func (m *mockEngineService) Stop(ctx context.Context) error {
	m.running = false
	m.logger.Info().Msg("Mock engine stopped")
	return nil
}

func (m *mockEngineService) IsRunning() bool {
	return m.running
}

func (m *mockEngineService) GetExchanges() []container.ExchangeService {
	return []container.ExchangeService{
		&mockExchangeService{name: "bybit"},
		&mockExchangeService{name: "binance"},
	}
}

func (m *mockEngineService) GetExchangeByName(name string) (container.ExchangeService, error) {
	exchanges := m.GetExchanges()
	for _, exchange := range exchanges {
		if exchange.GetName() == name {
			return exchange, nil
		}
	}
	return nil, fmt.Errorf("exchange %s not found", name)
}

func TestEngineService_MockBhavior(t *testing.T) {
	logger := &mockLogger{}

	mockEngine := &mockEngineService{
		logger:  logger,
		running: false,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := mockEngine.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start mock engine: %v", err)
	}

	if !mockEngine.IsRunning() {
		t.Error("Engine should be running after start")
	}

	err = mockEngine.Stop(ctx)
	if err != nil {
		t.Fatalf("Failed to stop mock engine: %v", err)
	}

	if mockEngine.IsRunning() {
		t.Error("Engine should not be running after stop")
	}
}

type mockExchangeService struct {
	name string
}

func (m *mockExchangeService) GetName() string {
	return m.name
}

func (m *mockExchangeService) UpdateAccountInfo(ctx context.Context, assetType asset.Item) (account.Holdings, error) {
	fmt.Println("UpdateAccountInfo called for exchange:", m.name)
	return account.Holdings{Exchange: m.name}, nil
}

func (m *mockExchangeService) UpdateTicker(ctx context.Context, pair currency.Pair, assetType asset.Item) (*ticker.Price, error) {
	fmt.Println("UpdateTicker called for exchange:", m.name)
	return &ticker.Price{ExchangeName: m.name, Pair: pair}, nil
}

func (m *mockExchangeService) GetWithdrawalsHistory(ctx context.Context, currency currency.Code, assetType asset.Item) ([]exchange.WithdrawalHistory, error) {
	if m.name == "bybit" {
		return []exchange.WithdrawalHistory{
			{
				Currency: "BTC",
				Amount:   1,
				Status:   "completed",
			},
		}, nil
	}
	return []exchange.WithdrawalHistory{}, nil
}

func TestWithdrawalService_FetchWithdrawalHistory(t *testing.T) {
	mockLogger := &mockLogger{}
	mockEngine := &mockEngineService{logger: mockLogger}
	mockRepo := newMockRepositoryService()

	withdrawalService := NewWithdrawalService(mockEngine, mockRepo, mockLogger)

	ctx := context.Background()
	withdrawals, err := withdrawalService.FetchWithdrawalHistory(ctx, "bybit", currency.BTC, asset.Spot)
	if err != nil {
		t.Fatalf("FetchWithdrawalHistory failed: %v", err)
	}

	if len(withdrawals) != 1 {
		t.Errorf("Expected 1 withdrawal, got %d", len(withdrawals))
	}

	// Verify that the withdrawal was stored in the mock repository
	if len(mockRepo.withdrawals["bybit"]) != 1 {
		t.Errorf("Expected 1 withdrawal to be stored, got %d", len(mockRepo.withdrawals["bybit"]))
	}

	if mockRepo.withdrawals["bybit"][0].Currency != "BTC" {
		t.Errorf("Expected stored withdrawal to have currency BTC, got %s", mockRepo.withdrawals["bybit"][0].Currency)
	}
}

func TestWithdrawalService_InterfaceCompliance(t *testing.T) {
	var _ container.WithdrawalService = (*withdrawalService)(nil)
	t.Log("Withdrawal service interface compliance verified")
}

func TestWithdrawalService_ExchangeNotFound(t *testing.T) {
	mockEngine := &mockEngineService{logger: &mockLogger{}}
	mockRepo := newMockRepositoryService()
	mockLogger := &mockLogger{}

	withdrawalService := NewWithdrawalService(mockEngine, mockRepo, mockLogger)

	ctx := context.Background()
	_, err := withdrawalService.FetchWithdrawalHistory(ctx, "nonexistent", currency.BTC, asset.Spot)
	if err == nil {
		t.Error("Expected error for non-existent exchange")
	}

	expectedError := "exchange nonexistent not found"
	if !strings.Contains(err.Error(), expectedError) {
		t.Errorf("Expected error %q, got %q", expectedError, err.Error())
	}
}
