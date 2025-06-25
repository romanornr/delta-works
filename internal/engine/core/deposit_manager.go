package core

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/romanornr/delta-works/internal/logger"
	"github.com/romanornr/delta-works/internal/repository"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/engine"
	"github.com/thrasher-corp/gocryptotrader/exchanges/bybit"
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

	b := bybit.Bybit{}
	err = b.VerifyAPICredentials(exch.GetDefaultCredentials())
	if err != nil {
		return fmt.Errorf("failed to verify API credentials for %s: %v", exchangeName, err)
	}

	accountType, err := b.FetchAccountType(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch account type for %s: %v", exchangeName, err)
	}

	logger.Info().Str("exchange", exchangeName).Str("accountType", accountType.String()).Msg("Account type")

	creds := exch.GetDefaultCredentials()
	b.SetCredentials(creds.Key, creds.Secret, creds.ClientID, "", creds.PEMKey, creds.OneTimePassword)

	// starttime 30 days ago
	startTime := time.Now().AddDate(0, 0, -30).Local()
	records, err := b.GetDepositRecords(ctx, currencyCode.String(), "", startTime, time.Now(), 0)
	if err != nil {
		return fmt.Errorf("failed to fetch deposit records for %s: %v", exchangeName, err)
	}

	for _, record := range records.Rows {
		logger.Info().Str("exchange", exchangeName).Str("transferID", record.TxID).Msg("Deposit found")
		logger.Info().Str("exchange", exchangeName).Str("amount", record.Amount).Msg("Amount")
	}

	os.Exit(0)

	//fundingHistory, err := exch.GetAccountFundingHistory(ctx)
	//if err != nil {
	//	return fmt.Errorf("failed to fetch funding history for %s: %v", exchangeName, err)
	//}
	//if fundingHistory == nil {
	//	return fmt.Errorf("exchange %s does not support funding history or no funding history found", exchangeName)
	//}
	//// Fetch deposit history
	//for _, deposit := range fundingHistory {
	//	logger.Info().Str("exchange", exchangeName).Str("transferID", deposit.TransferID).Msg("Deposit found")
	//	logger.Info().Str("exchange", exchangeName).Str("amount", deposit.Currency).Msg("Amount")
	//}

	return nil
}
