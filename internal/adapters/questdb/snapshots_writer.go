// Package questdb provides a QuestDB-backed implementation of the storage interfaces
package questdb

import (
	"context"
	"fmt"

	"github.com/romanornr/delta-works/internal/domain/portfolio"
)

// SnapshotStore implements storage.SnapshotStore using QuestDB
// Writes go through ILP; reads go through the Postgres wire protocol
type SnapshotStore struct {
	client *Client
}

// Write persists a portfolio snapshot via ILP
//
// We store snapshot metadata and holdings separately:
// - portfolio_snapshots: 1 row per snapshot
// - portfolio_positions: 1 row per asset position
func (s *SnapshotStore) Write(ctx context.Context, snap portfolio.Snapshot) error {
	// Write holdings before metadata. ILP writes are not transactional; with the
	// metadata row last, readers can treat metadata as the snapshot completion marker.
	for _, pos := range snap.Holdings {
		sender := s.client.sender.Table("portfolio_positions").
			Symbol("exchange", snap.Exchange).
			Symbol("account", snap.Account.String()).
			Symbol("asset", pos.Asset)

		// ILP NULLS are represented by omitting columns
		// For now we always include the decimals to keep ingestion logic simple
		sender = sender.
			DecimalColumnFromString("total", pos.Total.String()).
			DecimalColumnFromString("available", pos.Available.String()).
			DecimalColumnFromString("locked", pos.Locked.String()).
			DecimalColumnFromString("available_without_borrow", pos.AvailableWithoutBorrow.String()).
			DecimalColumnFromString("borrow", pos.Borrow.String()).
			DecimalColumnFromString("value_usd", pos.Value.String())

		if err := sender.At(ctx, snap.CapturedAt); err != nil {
			return fmt.Errorf("failed to write holding exchange %s, account %s, asset %s: %w", snap.Exchange, snap.Account.String(), pos.Asset, err)
		}
	}

	if err := s.client.sender.Table("portfolio_snapshots").
		Symbol("exchange", snap.Exchange).
		Symbol("account", snap.Account.String()).
		DecimalColumnFromString("total_value", snap.TotalValue.String()).
		At(ctx, snap.CapturedAt); err != nil {
		return fmt.Errorf("failed to write snapshot metadata: exchange=%s, account=%s: %w", snap.Exchange, snap.Account.String(), err)
	}

	return s.client.sender.Flush(ctx)
}
