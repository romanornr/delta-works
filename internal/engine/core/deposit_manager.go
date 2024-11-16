package core

import (
	"context"
	"fmt"
	"github.com/romanornr/delta-works/internal/logger"
	"github.com/romanornr/delta-works/internal/repository"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/engine"
)

// DepositManager manages the deposit-related operations and interactions with the QuestDB repository.
type DepositManager struct {
	instance *Instance
	repo     *repository.QuestDBRepository
}

// NewDepositManager creates and returns a new DepositManager instance along with a QuestDB repository.
// It takes an Instance pointer and a QuestDB configuration string as parameters.
// Returns a pointer to DepositManager and an error if the repository initialization fails.
func NewDepositManager(i *Instance, QuestDBConfig string) (*DepositManager, error) {
	repo, err := repository.NewQuestDBRepository(context.Background(), QuestDBConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create QuestDB repository: %w", err)
	}

	return &DepositManager{instance: i, repo: repo}, nil
}

func (dm *DepositManager) FetchDepositHistory(ctx context.Context, exchangeName string, currencyCode currency.Code) error {
	exch, err := engine.Bot.GetExchangeByName(exchangeName)
	if err != nil {
		return fmt.Errorf("exchange %s not found", exchangeName)
	}

	fundingHistory, err := exch.GetAccountFundingHistory(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch funding history for %s: %v", exchangeName, err)
	}
	if fundingHistory == nil {
		return fmt.Errorf("exchange %s does not support funding history or no funding history found", exchangeName)
	}
	// Fetch deposit history
	for _, deposit := range fundingHistory {
		logger.Info().Str("exchange", exchangeName).Str("transferID", deposit.TransferID).Msg("Deposit found")
		logger.Info().Str("exchange", exchangeName).Str("amount", deposit.Currency).Msg("Amount")
	}

	return nil
}
