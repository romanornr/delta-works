package services

import (
	"context"

	"github.com/romanornr/delta-works/internal/contracts"
	"github.com/romanornr/delta-works/internal/models"
	"github.com/romanornr/delta-works/internal/repository"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
)

type repositoryService struct {
	questDB *repository.QuestDBRepository
	logger  contracts.Logger
}

// NewRepositoryService creates a new repository service
func NewRepositoryService(questDBConfig string, logger contracts.Logger) (contracts.RepositoryService, error) {
	questDB, err := repository.NewQuestDBRepository(context.Background(), questDBConfig, logger)
	if err != nil {
		return nil, err
	}

	return &repositoryService{
		questDB: questDB,
		logger:  logger,
	}, nil
}

// InsertHoldings insert holdings into the database
func (r *repositoryService) InsertHoldings(ctx context.Context, holdings models.AccountHoldings) error {
	return r.questDB.InsertHoldings(ctx, holdings)
}

// StoreWithdrawal stores withdrawal history for a specific exchange
func (r *repositoryService) StoreWithdrawal(ctx context.Context, exchange string, withdrawals []exchange.WithdrawalHistory) error {
	return r.questDB.StoreWithdrawal(ctx, exchange, withdrawals)
}

// Close closes the repository service
func (r *repositoryService) Close(ctx context.Context) error {
	r.logger.Info().Msg("Repository service closed successfully")
	return nil
}
