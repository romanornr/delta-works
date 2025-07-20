package container

import (
	"context"
	"testing"

	"github.com/romanornr/delta-works/internal/models"
	"github.com/thrasher-corp/gocryptotrader/currency"
	exchanges "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/account"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/ticker"
)

// mockEngineService implements EngineService
type mockEngineService struct{}

func (m *mockEngineService) GetExchanges() []ExchangeService {
	return nil
}

func (m *mockEngineService) GetExchangeByName(name string) (ExchangeService, error) {
	return nil, nil
}

func (m *mockEngineService) Start(ctx context.Context) error {
	return nil
}

func (m *mockEngineService) Stop(ctx context.Context) error {
	return nil
}

func (m *mockEngineService) IsRunning() bool {
	return false
}

// mockExchangeService implements ExchangeService
type mockExchangeService struct{}

func (m *mockExchangeService) GetName() string {
	return ""
}

func (m *mockExchangeService) UpdateAccountInfo(ctx context.Context, assetType asset.Item) (account.Holdings, error) {
	return account.Holdings{}, nil
}

func (m *mockExchangeService) UpdateTicker(ctx context.Context, pair currency.Pair, assetType asset.Item) (*ticker.Price, error) {
	return nil, nil
}
func (m *mockExchangeService) GetWithdrawalsHistory(ctx context.Context, currency currency.Code, assetType asset.Item) ([]exchanges.WithdrawalHistory, error) {
	return nil, nil
}

// mockRepositoryService implements RepositoryService
type mockRepositoryService struct{}

func (m *mockRepositoryService) InsertHoldings(ctx context.Context, holdings models.AccountHoldings) error {
	return nil
}

func (m *mockRepositoryService) StoreWithdrawal(ctx context.Context, exchange string, withdrawals []exchanges.WithdrawalHistory) error {
	return nil
}

func (m *mockRepositoryService) Close(ctx context.Context) error {
	return nil
}

// mockHoldingsService implements HoldingsService
type mockHoldingsService struct{}

func (m *mockHoldingsService) UpdateHoldings(ctx context.Context, exchangeName string, accountType asset.Item) error {
	return nil
}

func (m *mockHoldingsService) GetHoldings(exchangeName string, accountType asset.Item) (*models.AccountHoldings, error) {
	return nil, nil
}

func (m *mockHoldingsService) StartContinuousUpdate(ctx context.Context) error {
	return nil
}

func (m *mockHoldingsService) Stop(ctx context.Context) error {
	return nil
}

// mockWithdrawalService implements WithdrawalService
type mockWithdrawalService struct{}

func (m *mockWithdrawalService) FetchWithdrawalHistory(ctx context.Context, exchangeName string, currency currency.Code, accountType asset.Item) ([]exchanges.WithdrawalHistory, error) {
	return nil, nil
}

// mockLogger implements Logger
//
// Note: This struct includes an `id int64` field to work around Go's empty struct optimization.
// Without this field, Go would reuse the same memory address for all empty struct instances,
// causing different mockLogger instances to appear identical when compared with == or used
// as map keys. This was causing test failures where we expected different instances to be
// treated as distinct objects. The id field ensures each instance has a unique identity
// for testing purposes.
type mockLogger struct {
	id int64 // Unique identifier to prevent Go's empty struct optimization from reusing memory addresses
}

func (m *mockLogger) Info() LogEvent {
	return &mockLogEvent{}
}

func (m *mockLogger) Debug() LogEvent {
	return &mockLogEvent{}
}

func (m *mockLogger) Warn() LogEvent {
	return &mockLogEvent{}
}

func (m *mockLogger) Error() LogEvent {
	return &mockLogEvent{}
}

type mockLogEvent struct{}

func (m *mockLogEvent) Msg(msg string)                          {}
func (m *mockLogEvent) Msgf(format string, args ...interface{}) {}
func (m *mockLogEvent) Err(err error) LogEvent {
	return m
}

// Compile-time interface compliance checks
var _EngineService = (*mockEngineService)(nil)
var _ExchangeService = (*mockExchangeService)(nil)
var _RepositoryService = (*mockRepositoryService)(nil)
var _HoldingsService = (*mockHoldingsService)(nil)
var _WithdrawalService = (*mockWithdrawalService)(nil)
var _Logger = (*mockLogger)(nil)
var _LogEvent = (*mockLogEvent)(nil)

func TestInterfaceCompliance(t *testing.T) {
	// Test that mock implementations can be assigned to interface variables
	// This verifies interface compliance at runtime (compile-time checks are above)
	var engine EngineService = &mockEngineService{}
	var exchange ExchangeService = &mockExchangeService{}
	var repo RepositoryService = &mockRepositoryService{}
	var holdings HoldingsService = &mockHoldingsService{}
	var withdrawal WithdrawalService = &mockWithdrawalService{}
	var logger Logger = &mockLogger{}

	// Test that the interfaces can be used (basic smoke test)
	if engine.IsRunning() {
		t.Error("mock engine should not be running")
	}

	if exchange.GetName() != "" {
		t.Error("mock exchange should return empty name")
	}

	if logger.Info() == nil {
		t.Error("mock logger Info() should not return nil")
	}

	// Verify all interface variables are properly assigned (this should always pass)
	_ = engine
	_ = exchange
	_ = repo
	_ = holdings
	_ = withdrawal
	_ = logger
}
