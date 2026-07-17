// Package postgres is the durable state adapter (ADR-0004: Postgres is
// truth). Migrations are embedded and applied at startup; queries are
// sqlc-generated from SQL in queries/.
package postgres

import (
	"context"
	"embed"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/romanornr/delta-works/internal/adapters/postgres/sqlcgen"
	"github.com/romanornr/delta-works/internal/config"
	"github.com/romanornr/delta-works/internal/domain/account"
	"github.com/romanornr/delta-works/internal/domain/instrument"
	"github.com/romanornr/delta-works/internal/ports"
	"github.com/romanornr/delta-works/internal/snapshot"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Connect opens a pool and applies pending migrations. The daemon is the
// single writer, so migrating at startup is safe.
func Connect(ctx context.Context, cfg config.Postgres) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("postgres: connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}
	if err := migrate(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

func migrate(ctx context.Context, pool *pgxpool.Pool) error {
	goose.SetBaseFS(migrations)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("postgres: goose dialect: %w", err)
	}
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close() //nolint:errcheck // stdlib wrapper over a pool we manage
	if err := goose.UpContext(ctx, db, "migrations"); err != nil {
		return fmt.Errorf("postgres: migrate: %w", err)
	}
	return nil
}

// CheckpointStore records and reads snapshot checkpoints.
type CheckpointStore struct {
	q *sqlcgen.Queries
}

var (
	_ ports.SnapshotRecorder = (*CheckpointStore)(nil)
	_ ports.SnapshotReader   = (*CheckpointStore)(nil)
)

// NewCheckpointStore builds a store over the pool.
func NewCheckpointStore(pool *pgxpool.Pool) *CheckpointStore {
	return &CheckpointStore{q: sqlcgen.New(pool)}
}

// RecordSnapshot stores one durable checkpoint.
func (s *CheckpointStore) RecordSnapshot(ctx context.Context, c snapshot.Checkpoint) error {
	return s.q.RecordSnapshot(ctx, sqlcgen.RecordSnapshotParams{
		ID:           c.ID,
		Venue:        string(c.Account.Venue),
		AccountType:  string(c.Account.Type),
		TakenAt:      c.TakenAt,
		BalanceCount: int32(c.BalanceCount), //nolint:gosec // balance counts are tiny
		Status:       string(c.Status),
		Error:        c.Error,
	})
}

// LastSnapshot returns the most recent checkpoint for an account.
func (s *CheckpointStore) LastSnapshot(ctx context.Context, ref account.Ref) (snapshot.Checkpoint, error) {
	row, err := s.q.LastSnapshot(ctx, sqlcgen.LastSnapshotParams{
		Venue:       string(ref.Venue),
		AccountType: string(ref.Type),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return snapshot.Checkpoint{}, ports.ErrNotFound
	}
	if err != nil {
		return snapshot.Checkpoint{}, fmt.Errorf("postgres: last snapshot: %w", err)
	}
	return snapshot.Checkpoint{
		ID: row.ID,
		Account: account.Ref{
			Venue: instrument.VenueID(row.Venue),
			Type:  account.Type(row.AccountType),
		},
		TakenAt:      row.TakenAt,
		BalanceCount: int(row.BalanceCount),
		Status:       snapshot.Status(row.Status),
		Error:        row.Error,
	}, nil
}

// Health reports pool connectivity for /readyz.
type Health struct {
	pool *pgxpool.Pool
}

// NewHealth builds the Postgres readiness check.
func NewHealth(pool *pgxpool.Pool) *Health { return &Health{pool: pool} }

// Name implements ports.HealthChecker.
func (h *Health) Name() string { return "postgres" }

// Check implements ports.HealthChecker.
func (h *Health) Check(ctx context.Context) error { return h.pool.Ping(ctx) }
