package ports

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/romanornr/delta-works/internal/domain/account"
	"github.com/romanornr/delta-works/internal/domain/marketdata"
)

// ErrNotFound is returned by stores when a record does not exist.
var ErrNotFound = errors.New("not found")

// SeriesWriter appends time-series observations (QuestDB). This data is
// analytics, never accounting truth (ADR-0004).
type SeriesWriter interface {
	WriteBalanceSnapshot(ctx context.Context, s account.Snapshot) error
	WriteTicker(ctx context.Context, t marketdata.Ticker) error
	// Flush blocks until previously written rows are durably accepted by
	// the store. Checkpoints must only be recorded after a successful Flush.
	Flush(ctx context.Context) error
}

// SnapshotCheckpoint is the durable record that a balance snapshot reached
// the time-series store. It is the Postgres-side anchor for gap detection.
type SnapshotCheckpoint struct {
	ID           uuid.UUID
	Account      account.Ref
	TakenAt      time.Time
	BalanceCount int
	Status       CheckpointStatus
	Error        string
}

// CheckpointStatus classifies a snapshot attempt.
type CheckpointStatus string

// Checkpoint statuses.
const (
	CheckpointOK     CheckpointStatus = "ok"
	CheckpointFailed CheckpointStatus = "failed"
)

// CheckpointStore records snapshot checkpoints (Postgres).
type CheckpointStore interface {
	RecordSnapshot(ctx context.Context, c SnapshotCheckpoint) error
	// LastSnapshot returns the most recent checkpoint for an account, or
	// ErrNotFound.
	LastSnapshot(ctx context.Context, ref account.Ref) (SnapshotCheckpoint, error)
}
