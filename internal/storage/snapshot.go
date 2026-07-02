package storage

import (
	"context"
	"time"

	"github.com/romanornr/delta-works/internal/domain/portfolio"
)

// SnapshotWriter defines write operations for portfolio snapshots
type SnapshotWriter interface {
	Write(ctx context.Context, snap portfolio.Snapshot) error
}

// SnapshotReader defines read operations for portfolio snapshots
type SnapshotReader interface {
	Latest(ctx context.Context, exchange string, account portfolio.AccountType) (*portfolio.Snapshot, error)
	Range(ctx context.Context, exchange string, account portfolio.AccountType, from, to time.Time) ([]portfolio.Snapshot, error)
}

// SnapshotStore combines read and write operations
type SnapshotStore interface {
	SnapshotWriter
	SnapshotReader
}
