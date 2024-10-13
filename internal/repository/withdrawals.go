package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/romanornr/delta-works/internal/logger"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"strings"
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
	timestampColumnName       = "timestamp"
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

	lastTimeStamp, err := q.getLastWithdrawalTimestamp(ctx, exchangeName)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			lastTimeStamp = time.Time{} // No previous withdrawals found, start from zero time
		}
		return fmt.Errorf("failed to get last withdrawal timestamp: %w", err)
	}

	var insertCount int
	var errs []error
	for _, withdrawal := range withdrawals {

		// Skip if the withdrawal is older or equal to the last stored withdrawal
		if !withdrawal.Timestamp.After(lastTimeStamp) {
			continue
		}

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
			errs = append(errs, fmt.Errorf("failed to store withdrawal data: %s: %w", withdrawal.TransferID, err))
			continue
		}
		insertCount++
	}

	if insertCount > 0 {
		logger.Info().
			Str("exchange", exchangeName).
			Int("records", insertCount).
			Msg("stored withdrawal data")
		if err := q.sender.Flush(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to flush data: %w", err))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

// getLastWithdrawalTimestamp retrieves the most recent withdrawal timestamp for a given exchange.
func (q *QuestDBRepository) getLastWithdrawalTimestamp(ctx context.Context, exchangeName string) (time.Time, error) {
	// QuestDB doesn't support parameterized queries in the same way as other SQL databases. We need to use string formatting
	// TODO figure out prepared statements in QuestDB to prevent SQL injection
	query := fmt.Sprintf("SELECT MAX(%s) FROM %s WHERE %s = '%s'",
		timestampColumnName, tableNameWithdrawals, exchangeColumnName, exchangeName)

	var lastTimestamp sql.NullTime
	err := q.db.QueryRowContext(ctx, query).Scan(&lastTimestamp)
	if err != nil {
		if strings.Contains(err.Error(), "table does not exist") {
			logger.Info().Msg("Withdrawals table does not exist. This might be the initial sync.")
			return time.Time{}, nil // Return zero time for initial sync
		}
		if err == sql.ErrNoRows {
			return time.Time{}, nil // No withdrawals found, return zero time
		}
		logger.Error().Err(err).Str("exchange", exchangeName).Msg("failed to get last withdrawal timestamp")
		return time.Time{}, fmt.Errorf("failed to get last withdrawal timestamp: %w", err)
	}

	if lastTimestamp.Valid {
		return lastTimestamp.Time, nil // return the last timestamp
	}

	return time.Time{}, nil
}
