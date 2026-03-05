// Package questdb provides a QuestDB-backed implementation of the storage interfaces.
//
// This file contains the storage.Store facade used by application services.
package questdb

import (
	"context"

	"github.com/romanornr/delta-works/internal/storage"
)

// Store is the top-level QuestDB storage facade
// It delegates snapshot and transfer operations to dedicated sub-stores that
// share the same client.
type Store struct {
	client    *Client
	snapshots *SnapshotStore
	transfers *TransferStore
}

// NewStore returns a storage.Store backed by QuestDB
func NewStore(client *Client) storage.Store {
	return &Store{
		client:    client,
		snapshots: &SnapshotStore{client: client},
		transfers: &TransferStore{client: client},
	}
}

// Snapshots returns the snapshot read/write sub-store
func (s *Store) Snapshots() storage.SnapshotStore {
	return s.snapshots
}

// Transfers returns the transfer read/write sub-store
func (s *Store) Transfers() storage.TransferStore {
	return s.transfers
}

// Close shuts down the underlying QuestDB client connections
func (s *Store) Close(ctx context.Context) error {
	return s.client.Close(ctx)
}

// Ping checks whether the QuestDB Postgres connection is alive
func (s *Store) Ping(ctx context.Context) error {
	return s.client.Ping(ctx)
}
