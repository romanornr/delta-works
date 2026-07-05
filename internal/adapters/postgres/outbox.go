package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/romanornr/delta-works/internal/adapters/postgres/sqlcgen"
	"github.com/romanornr/delta-works/internal/ports"
)

// OutboxStore drains the transactional outbox (ADR-0008). Rows are
// written by other stores in their own transactions; this store only
// claims, publishes and cleans up.
type OutboxStore struct {
	pool *pgxpool.Pool
	q    *sqlcgen.Queries
}

var _ ports.OutboxStore = (*OutboxStore)(nil)

// NewOutboxStore returns an OutboxStore backed by pool.
func NewOutboxStore(pool *pgxpool.Pool) *OutboxStore {
	return &OutboxStore{pool: pool, q: sqlcgen.New(pool)}
}

// PublishPending claims up to limit rows with FOR UPDATE SKIP LOCKED,
// publishes each in id order, and marks the batch published in the same
// transaction. A publish error rolls the batch back for the next poll.
// Holding the row locks across publish is safe because the in-process bus
// never blocks (ADR-0005).
func (s *OutboxStore) PublishPending(ctx context.Context, limit int, publish func(ports.OutboxMessage) error) (int, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("postgres: begin outbox publish: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := s.q.WithTx(tx)
	rows, err := q.ClaimUnpublishedOutbox(ctx, int64(limit))
	if err != nil {
		return 0, fmt.Errorf("postgres: claim outbox rows: %w", err)
	}
	if len(rows) == 0 {
		return 0, nil
	}

	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if err := publish(ports.OutboxMessage{
			ID:        row.ID,
			Subject:   row.Subject,
			Payload:   row.Payload,
			CreatedAt: row.CreatedAt,
		}); err != nil {
			return 0, fmt.Errorf("postgres: publish outbox row %d: %w", row.ID, err)
		}
		ids = append(ids, row.ID)
	}

	if err := q.MarkOutboxPublished(ctx, ids); err != nil {
		return 0, fmt.Errorf("postgres: mark outbox published: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("postgres: commit outbox publish: %w", err)
	}
	return len(rows), nil
}

// DeletePublishedBefore removes rows published before cutoff.
func (s *OutboxStore) DeletePublishedBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	n, err := s.q.DeleteOutboxPublishedBefore(ctx, pgtype.Timestamptz{Time: cutoff, Valid: true})
	if err != nil {
		return 0, fmt.Errorf("postgres: delete published outbox rows: %w", err)
	}
	return n, nil
}

// UnpublishedStats reports the backlog size and the creation time of the
// oldest unpublished row (now when the backlog is empty).
func (s *OutboxStore) UnpublishedStats(ctx context.Context) (int64, time.Time, error) {
	row, err := s.q.OutboxUnpublishedStats(ctx)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("postgres: outbox stats: %w", err)
	}
	return row.Unpublished, row.Oldest, nil
}
