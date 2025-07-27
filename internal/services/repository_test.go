package services

import (
	"context"
	"testing"
	"time"

	"github.com/romanornr/delta-works/internal/contracts"
	"github.com/romanornr/delta-works/internal/models"
	"github.com/shopspring/decimal"
	"github.com/thrasher-corp/gocryptotrader/currency"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
)

func TestRepositoryService_interfaceCompliance(t *testing.T) {
	var _ contracts.RepositoryService = (*repositoryService)(nil)
	t.Log("Repository service interface compliance verified")
}

func TestRepositoryService_Creation(t *testing.T) {
	logger := &mockLogger{}

	// Test with mock config will faill in real environment
	questDBConfig := "mock-config"

	_, err := NewRepositoryService(questDBConfig, logger)
	if err == nil {
		t.Log("Repository service created successfully (unexpected in test environment)")
	} else {
		t.Logf("Repository service creation failed as expected in test environment: %v", err)
	}
}

func TestMockRepositoryService_Operations(t *testing.T) {
	mockRepo := newMockRepositoryService()
	ctx := context.Background()

	// Test InsertHoldings
	holdings := models.AccountHoldings{
		ExchangeName: "bybit",
		AccountType:  asset.Spot,
		Balances: map[currency.Code]models.AssetBalance{
			currency.BTC: {
				Currency: currency.BTC,
				Total:    decimal.NewFromFloat(1.5),
				USDValue: decimal.NewFromFloat(7500),
			},
		},
		TotalUSDValue: decimal.NewFromFloat(7500),
		LastUpdated:   time.Now(),
	}

	err := mockRepo.InsertHoldings(ctx, holdings)
	if err != nil {
		t.Fatalf("InsertHoldings failed: %v", err)
	}

	// Verify holdings were stored
	storedHoldings := mockRepo.GetStoredHoldings()
	if len(storedHoldings) != 1 {
		t.Errorf("Expected 1 stored holding, got %d", len(storedHoldings))
	}

	if storedHoldings[0].ExchangeName != "bybit" {
		t.Errorf("Expected exchange name 'bybit', got %s", storedHoldings[0].ExchangeName)
	}

	withdrawals := []exchange.WithdrawalHistory{
		{
			Status:     "completed",
			TransferID: "test-123",
			Currency:   currency.BTC.String(),
			Amount:     decimal.NewFromFloat(0.1).InexactFloat64(),
		},
	}

	err = mockRepo.StoreWithdrawal(ctx, "bybit", withdrawals)
	if err != nil {
		t.Fatalf("StoreWithdrawal failed: %v", err)
	}

	err = mockRepo.Close(ctx)
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}
