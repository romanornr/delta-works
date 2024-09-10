package core

import (
	"context"
	"fmt"
	"github.com/romanornr/delta-works/internal/repository"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/engine"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
)

type WithdrawalManager struct {
	instance *Instance
	repo     *repository.QuestDBRepository
}

func NewWithdrawalManager(i *Instance, QuestDBConfig string) (*WithdrawalManager, error) {
	repo, err := repository.NewQuestDBRepository(context.Background(), QuestDBConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create QuestDB repository: %w", err)
	}
	return &WithdrawalManager{
		instance: i,
		repo:     repo,
	}, nil
}

func (wm *WithdrawalManager) FetchWithdrawalHistory(ctx context.Context, exchangeName string, currency currency.Code, accountType asset.Item) ([]exchange.WithdrawalHistory, error) {
	exch, err := engine.Bot.ExchangeManager.GetExchangeByName(exchangeName)
	if err != nil {
		return nil, fmt.Errorf("exchange %s not found", exchangeName)
	}

	//// or
	//exch, err := engine.Bot.GetExchangeByName(exchangeName)
	//if err != nil {
	//	return nil, fmt.Errorf("exchange %s not found", exchangeName)
	//}

	history, err := exch.GetWithdrawalsHistory(ctx, currency, accountType)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch withdrawal history for %s %s: %v", exchangeName, currency, err)
	}

	return history, nil

}
