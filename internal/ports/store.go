package ports

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/romanornr/delta-works/internal/domain/account"
	"github.com/romanornr/delta-works/internal/domain/instrument"
	"github.com/romanornr/delta-works/internal/domain/marketdata"
	"github.com/romanornr/delta-works/internal/domain/order"
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

// StoredOrder is the persisted state of an order. Zero-valued
// AvgFillPrice, VenueOrderID and CancelRequestedAt mean not yet known.
type StoredOrder struct {
	ClientOrderID     order.ClientOrderID
	BotID             string
	Instrument        instrument.Instrument
	Side              order.Side
	Type              order.Type
	Price             decimal.Decimal
	Qty               decimal.Decimal
	FilledQty         decimal.Decimal
	AvgFillPrice      decimal.Decimal
	Status            order.Status
	VenueOrderID      string
	CancelRequestedAt time.Time
	Reason            string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// OrderStore persists order state per the M2 state machine
// (docs/specs/m2-oms.md). Every write goes through a transaction that
// also inserts the matching outbox rows (ADR-0008).
type OrderStore interface {
	// CreatePending inserts the order in status pending before the venue
	// submit. Idempotent: re-inserting the same ClientOrderID is a no-op.
	CreatePending(ctx context.Context, req order.Request) error
	// ApplyEvent applies a venue event: transition row, fill row and
	// outbox rows in one transaction. Idempotent and order-independent.
	// Returns ErrNotFound when the order is unknown.
	ApplyEvent(ctx context.Context, source order.Source, ev order.Event) (order.Decision, error)
	// GetOrder returns the stored order, or ErrNotFound.
	GetOrder(ctx context.Context, id order.ClientOrderID) (StoredOrder, error)
	// MarkCancelRequested stamps the cancel intent once; later calls keep
	// the first timestamp. Returns ErrNotFound for unknown orders.
	MarkCancelRequested(ctx context.Context, id order.ClientOrderID, at time.Time) error
}

// OutboxMessage is one unpublished outbox row.
type OutboxMessage struct {
	ID        int64
	Subject   string
	Payload   []byte // jsonb
	CreatedAt time.Time
}

// OutboxStore drains the transactional outbox (ADR-0008).
type OutboxStore interface {
	// PublishPending claims up to limit unpublished rows in id order,
	// calls publish for each, and marks them published in the same
	// transaction. A publish error aborts the batch so the rows are
	// retried on the next poll: delivery is at-least-once. Returns the
	// number of rows published.
	PublishPending(ctx context.Context, limit int, publish func(OutboxMessage) error) (int, error)
	// DeletePublishedBefore removes rows published before cutoff and
	// returns how many were deleted.
	DeletePublishedBefore(ctx context.Context, cutoff time.Time) (int64, error)
	// UnpublishedStats reports the backlog: row count and the creation
	// time of the oldest unpublished row (now when the backlog is empty).
	UnpublishedStats(ctx context.Context) (rows int64, oldest time.Time, err error)
}
