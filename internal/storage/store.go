package storage

import "context"

// Store combines all storage interfaces with lifecycle management
type Store interface {
	Snapshots() SnapshotStore
	Transfers() TransferStore

	Close(ctx context.Context) error
	Ping(ctx context.Context) error
}
