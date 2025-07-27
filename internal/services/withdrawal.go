package services

import (
	"context"
	"fmt"

	"github.com/romanornr/delta-works/internal/contracts"
	"github.com/thrasher-corp/gocryptotrader/currency"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
)

// WithdrawalService handles withdrawal operations with dependency injection
type withdrawalService struct {
	engine contracts.EngineService
	repo   contracts.RepositoryService
	logger contracts.Logger
}

// NewWithdrawalService creates a new withdrawal service
func NewWithdrawalService(engine contracts.EngineService, repo contracts.RepositoryService, logger contracts.Logger) contracts.WithdrawalService {
	return &withdrawalService{
		engine: engine,
		repo:   repo,
		logger: logger,
	}
}

func (w *withdrawalService) FetchWithdrawalHistory(ctx context.Context, exchangeName string, curr currency.Code, accountType asset.Item) ([]exchange.WithdrawalHistory, error) {
	exch, err := w.engine.GetExchangeByName(exchangeName)
	if err != nil {
		return nil, fmt.Errorf("exchange %s not found", exchangeName)
	}

	history, err := exch.GetWithdrawalsHistory(ctx, curr, accountType)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch withdrawal history for %s %s: %v", exchangeName, curr.String(), err)
	}

	if err := w.repo.StoreWithdrawal(ctx, exchangeName, history); err != nil {
		return nil, fmt.Errorf("failed to store withdrawal history for %s %s: %v", exchangeName, curr.String(), err)
	}

	w.logger.Info().Msgf("Withdrawal history stored for %s %s", exchangeName, curr.String())
	return history, nil
}
