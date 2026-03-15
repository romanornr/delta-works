package questdb

import (
	"context"
	"fmt"

	"github.com/romanornr/delta-works/internal/domain/transfer"
)

// writeOne queues a single transfer row on the QuestDB ILP sender without flushing.
func (s *TransferStore) writeOne(ctx context.Context, t transfer.Transfer) error {
	sender := s.client.sender.Table("transfers").
		Symbol("exchange", t.Exchange).
		Symbol("direction", string(t.Direction)).
		Symbol("transfer_type", string(t.Type)).
		Symbol("asset", t.Asset).
		DecimalColumnFromString("amount", t.Amount.String()).
		DecimalColumnFromString("fee", t.Fee.String()).
		Symbol("status", string(t.Status)).
		Symbol("network", t.Network).
		StringColumn("tx_hash", t.TxHash).
		StringColumn("address", t.Address).
		StringColumn("bank_to", t.BankTo).
		StringColumn("description", t.Description)

	if err := sender.At(ctx, t.CompletedAt); err != nil {
		return fmt.Errorf("failed to write transfer for exchange %s, direction %s, and id %s: %w", t.Exchange, t.Direction, t.ID, err)
	}

	return nil
}

func (s *TransferStore) Write(ctx context.Context, t transfer.Transfer) error {
	if err := s.writeOne(ctx, t); err != nil {
		return err
	}

	if err := s.client.sender.Flush(ctx); err != nil {
		return fmt.Errorf("failed to flush transfer write for exchange %s, direction %s, and id %s: %w", t.Exchange, t.Direction, t.ID, err)
	}

	return nil
}

// WriteBatch persists a batch of transfers
func (s *TransferStore) WriteBatch(ctx context.Context, transfers []transfer.Transfer) error {
	for _, t := range transfers {
		if err := s.writeOne(ctx, t); err != nil {
			return err
		}
	}

	if err := s.client.sender.Flush(ctx); err != nil {
		return fmt.Errorf("failed to flush transfer batch with %d transfers: %w", len(transfers), err)
	}

	return nil
}
