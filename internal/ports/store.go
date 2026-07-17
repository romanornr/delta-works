package ports

import (
	"context"
	"errors"
	"time"

	"github.com/romanornr/delta-works/internal/domain/account"
	"github.com/romanornr/delta-works/internal/domain/instrument"
	"github.com/romanornr/delta-works/internal/domain/marketdata"
	"github.com/romanornr/delta-works/internal/domain/order"
	"github.com/romanornr/delta-works/internal/events"
	"github.com/romanornr/delta-works/internal/snapshot"
)

// ErrNotFound is returned by stores when a record does not exist.
var ErrNotFound = errors.New("not found")

// BalanceSeriesWriter appends analytics-only balance rows and durably flushes them (ADR-0004).
type BalanceSeriesWriter interface {
	WriteBalanceSnapshot(ctx context.Context, s account.Snapshot) error
	// Flush blocks until previously written rows are durably accepted by
	// the store. Checkpoints must only be recorded after a successful Flush.
	Flush(ctx context.Context) error
}

// TickerSeriesWriter appends analytics-only ticker rows and durably flushes them (ADR-0004).
type TickerSeriesWriter interface {
	WriteTicker(ctx context.Context, t marketdata.Ticker) error
	Flush(ctx context.Context) error
}

// SnapshotRecorder records durable snapshot checkpoints.
type SnapshotRecorder interface {
	RecordSnapshot(ctx context.Context, checkpoint snapshot.Checkpoint) error
}

// SnapshotReader returns the most recent checkpoint for an account, or
// ErrNotFound.
type SnapshotReader interface {
	LastSnapshot(ctx context.Context, ref account.Ref) (snapshot.Checkpoint, error)
}

// OrderCommandStore persists order commands before venue calls.
type OrderCommandStore interface {
	// CreatePending inserts the order in status pending before the venue
	// submit. Idempotent: re-inserting the same ClientOrderID reports false.
	CreatePending(ctx context.Context, req order.Request) (bool, error)
	// GetOrder returns the stored order, or ErrNotFound.
	GetOrder(ctx context.Context, id order.ClientOrderID) (order.Record, error)
	// MarkCancelRequested stamps the cancel intent once; later calls keep
	// the first timestamp. Returns ErrNotFound for unknown orders.
	MarkCancelRequested(ctx context.Context, id order.ClientOrderID, at time.Time) error
}

// OrderEventStore applies venue events to durable order state.
type OrderEventStore interface {
	// ApplyEvent applies one venue event: transition row, fill row, ledger
	// posting and outbox rows in a single transaction. Idempotent and
	// order-independent. Returns ErrNotFound when the order is unknown.
	ApplyEvent(ctx context.Context, source order.Source, ev order.Event) (order.ApplyResult, error)
}

// OrderReconcileStore reads the local order state needed for convergence.
type OrderReconcileStore interface {
	// GetOrder returns the stored order, or ErrNotFound.
	GetOrder(ctx context.Context, id order.ClientOrderID) (order.Record, error)
	// ListActiveOrders returns every non-terminal order (pending, open,
	// partially_filled) for one venue.
	ListActiveOrders(ctx context.Context, venue instrument.VenueID) ([]order.Record, error)
}

// OrderQueryStore serves keyset-paginated order reads.
type OrderQueryStore interface {
	// ListOrders returns at most query.Limit+1 rows so the caller can
	// derive a next-page token.
	ListOrders(ctx context.Context, query order.Query) ([]order.Record, error)
}

// OutboxStore drains the transactional outbox (ADR-0008).
type OutboxStore interface {
	// PublishPending claims up to limit unpublished rows in id order,
	// calls publish for each, and marks them published in the same
	// transaction. A publish error aborts the batch so the rows are
	// retried on the next poll: delivery is at-least-once. Returns the
	// number of rows published.
	PublishPending(ctx context.Context, limit int, publish func(events.OutboxMessage) error) (int, error)
	// DeletePublishedBefore removes rows published before cutoff and
	// returns how many were deleted.
	DeletePublishedBefore(ctx context.Context, cutoff time.Time) (int64, error)
	// UnpublishedStats reports the backlog: row count and the creation
	// time of the oldest unpublished row (now when the backlog is empty).
	UnpublishedStats(ctx context.Context) (rows int64, oldest time.Time, err error)
}
