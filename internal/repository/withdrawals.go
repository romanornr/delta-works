package repository

import (
	"context"
	"fmt"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
)

// StoreWithdrawal saves the given withdrawal record into the "withdrawals" table in QuestDB.
// It records various details such as exchange, status, transfer ID, and amount.
// If storing the data or flushing the data to QuestDB fails, an error is returned.
func (q *QuestDBRepository) StoreWithdrawal(ctx context.Context, exchangeName string, withdrawals []exchange.WithdrawalHistory) error {
	if len(withdrawals) == 0 {
		return nil
	}

	var insertCount int
	for _, withdrawal := range withdrawals {
		err := q.sender.
			Table("withdrawals").
			// Symbol columns first
			Symbol("exchange", exchangeName).
			Symbol("status", withdrawal.Status).
			Symbol("transfer_id", withdrawal.TransferID).
			Symbol("description", withdrawal.Description).
			Symbol("currency", withdrawal.Currency).
			Symbol("transfer_type", withdrawal.TransferType).
			Symbol("crypto_to_address", withdrawal.CryptoToAddress).
			Symbol("crypto_tx_id", withdrawal.CryptoTxID).
			Symbol("crypto_chain", withdrawal.CryptoChain).
			Symbol("bank_to", withdrawal.BankTo).
			// Float columns after
			Float64Column("amount", withdrawal.Amount).
			Float64Column("fee", withdrawal.Fee).
			// Timestamp last
			At(ctx, withdrawal.Timestamp)

		if err != nil {
			return fmt.Errorf("failed to store withdrawal data: %v", err)
		}
		insertCount++
	}

	if insertCount > 0 {
		if err := q.sender.Flush(ctx); err != nil {
			return fmt.Errorf("failed to flush data: %w", err)
		}
	}

	return nil
}

//func (q *QuestDBRepository) getLastWithdrawalTimestamp(ctx context.Context, exchangeName string) (time.Time, error) {
//
//}
