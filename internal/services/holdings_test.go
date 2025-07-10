package services

import (
	"context"
	"testing"
	"time"

	"github.com/romanornr/delta-works/internal/container"
	"github.com/romanornr/delta-works/internal/models"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
)

type mockRepositoryService struct {
	holdings    []models.AccountHoldings
	withdrawals map[string][]exchange.WithdrawalHistory
}

func newMockRepositoryService() *mockRepositoryService {
	return &mockRepositoryService{
		holdings:    make([]models.AccountHoldings, 0),
		withdrawals: make(map[string][]exchange.WithdrawalHistory),
	}
}

func (m *mockRepositoryService) InsertHoldings(ctx context.Context, holdings models.AccountHoldings) error {
	m.holdings = append(m.holdings, holdings)
	return nil
}

func (m *mockRepositoryService) StoreWithdrawal(ctx context.Context, exchange string, withdrawals []exchange.WithdrawalHistory) error {
	m.withdrawals[exchange] = withdrawals
	return nil
}

func (m *mockRepositoryService) Close(ctx context.Context) error {
	return nil
}

func (m *mockRepositoryService) GetStoredHoldings() []models.AccountHoldings {
	return m.holdings
}

func TestHoldingsService_UpdateHoldings(t *testing.T) {
	mockEngine := &mockEngineService{logger: &mockLogger{}}
	mockRepo := newMockRepositoryService()
	mockLogger := &mockLogger{}

	holdingsService := NewHoldingsService(mockEngine, mockRepo, mockLogger)

	// Test update holdings
	ctx := context.Background()
	err := holdingsService.UpdateHoldings(ctx, "bybit", asset.Spot)
	if err != nil {
		t.Fatalf("UpdateHoldings failed: %v", err)
	}

	// Verify holdings were stored in repository
	storedHoldings := mockRepo.GetStoredHoldings()
	if len(storedHoldings) != 1 {
		t.Errorf("Expected 1 stored holding, got %d", len(storedHoldings))
	}

	if storedHoldings[0].ExchangeName != "bybit" {
		t.Errorf("Expected exchange name 'bybit', got %s", storedHoldings[0].ExchangeName)
	}

	if storedHoldings[0].AccountType != asset.Spot {
		t.Errorf("Expected account type %v, got %v", asset.Spot, storedHoldings[0].AccountType)
	}
}

func TestHoldingsService_GetHoldings(t *testing.T) {
	mockEngine := &mockEngineService{logger: &mockLogger{}}
	mockRepo := newMockRepositoryService()
	mockLogger := &mockLogger{}

	holdingsService := NewHoldingsService(mockEngine, mockRepo, mockLogger)
	ctx := context.Background()

	err := holdingsService.UpdateHoldings(ctx, "bybit", asset.Spot)
	if err != nil {
		t.Fatalf("UpdateHoldings failed: %v", err)
	}

	holdings, err := holdingsService.GetHoldings("bybit", asset.Spot)
	if err != nil {
		t.Fatalf("GetHoldings failed: %v", err)
	}

	if holdings.ExchangeName != "bybit" {
		t.Errorf("Expected exchange name 'bybit', got %s", holdings.ExchangeName)
	}
}

func TestHoldingsService_StartStopContinuousUpdate(t *testing.T) {
	mockEngine := &mockEngineService{logger: &mockLogger{}}
	mockRepo := newMockRepositoryService()
	mockLogger := &mockLogger{}

	// create holdings service
	holdingsService := NewHoldingsService(mockEngine, mockRepo, mockLogger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// start continuous update
	err := holdingsService.StartContinuousUpdate(ctx)
	if err != nil {
		t.Fatalf("StartContinuousUpdate failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// stop continuous update
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()

	err = holdingsService.Stop(stopCtx)
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestHoldingsService_InterfaceCompliance(t *testing.T) {
	var _ container.HoldingsService = (*holdingsService)(nil)
	t.Log("Holdings service interface compliance verified")
}
