package repository

import (
	"context"
	"fmt"
	"github.com/romanornr/delta-works/internal/logger"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"time"
)

const (
	tableNameWithdrawals      = "withdrawals"
	exchangeColumnName        = "exchange"
	statusColumnName          = "status"
	transferIDColumnName      = "transfer_id"
	descriptionColumnName     = "description"
	currencyColumnName        = "currency"
	transferTypeColumnName    = "transfer_type"
	cryptoToAddressColumnName = "crypto_to_address"
	cryptoTxIDColumnName      = "crypto_tx_id"
	cryptoChainColumnName     = "crypto_chain"
	bankToColumnName          = "bank_to"
	amountColumnName          = "amount"
	feeColumnName             = "fee"
)

// StoreWithdrawal saves the given withdrawal record into the "withdrawals" table in QuestDB.
// It records various details such as exchange, status, transfer ID, and amount.
// If storing the data or flushing the data to QuestDB fails, an error is returned.
func (q *QuestDBRepository) StoreWithdrawal(ctx context.Context, exchangeName string, withdrawals []exchange.WithdrawalHistory) error {
	if len(withdrawals) == 0 {
		return nil
	}

	// Create a context with a timeout of 10 seconds to prevent indefinite blocking
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var insertCount int
	for _, withdrawal := range withdrawals {
		err := q.sender.
			Table(tableNameWithdrawals).
			// Symbol columns first
			Symbol(exchangeColumnName, exchangeName).
			Symbol(statusColumnName, withdrawal.Status).
			Symbol(transferIDColumnName, withdrawal.TransferID).
			Symbol(descriptionColumnName, withdrawal.Description).
			Symbol(currencyColumnName, withdrawal.Currency).
			Symbol(transferTypeColumnName, withdrawal.TransferType).
			Symbol(cryptoToAddressColumnName, withdrawal.CryptoToAddress).
			Symbol(cryptoTxIDColumnName, withdrawal.CryptoTxID).
			Symbol(cryptoChainColumnName, withdrawal.CryptoChain).
			Symbol(bankToColumnName, withdrawal.BankTo).
			// Float columns after
			Float64Column(amountColumnName, withdrawal.Amount).
			Float64Column(feeColumnName, withdrawal.Fee).
			// Timestamp last
			At(ctx, withdrawal.Timestamp)

		if err != nil {
			return fmt.Errorf("failed to store withdrawal data: %v", err)
		}
		insertCount++
	}

	if insertCount > 0 {
		logger.Info().
			Str("exchange", exchangeName).
			Int("records", insertCount).
			Msg("stored withdrawal data")
		if err := q.sender.Flush(ctx); err != nil {
			return fmt.Errorf("failed to flush data: %w", err)
		}
	}

	return nil
}

//func (q *QuestDBRepository) getLastWithdrawalTimestamp(ctx context.Context, exchangeName string) (time.Time, error) {
//
//}
