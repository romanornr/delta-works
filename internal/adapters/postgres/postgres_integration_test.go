//go:build integration

package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/romanornr/delta-works/internal/config"
	"github.com/romanornr/delta-works/internal/domain/account"
	"github.com/romanornr/delta-works/internal/ports"
	"github.com/romanornr/delta-works/internal/snapshot"
)

func startPostgres(t *testing.T) config.Postgres {
	t.Helper()
	ctx := context.Background()
	container, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("delta"),
		tcpostgres.WithUsername("delta"),
		tcpostgres.WithPassword("delta"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(context.Background()); err != nil {
			t.Logf("terminate container: %v", err)
		}
	})
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	return config.Postgres{DSN: dsn}
}

func TestMigrationsAndCheckpointRoundTrip(t *testing.T) {
	ctx := context.Background()
	pool, err := Connect(ctx, startPostgres(t)) // Connect runs goose migrations
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer pool.Close()

	store := NewCheckpointStore(pool)
	ref := account.Ref{Venue: "bybit", Type: account.TypeSpot}

	if _, err := store.LastSnapshot(ctx, ref); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("empty table: expected ErrNotFound, got %v", err)
	}

	older := snapshot.Checkpoint{
		ID: uuid.New(), Account: ref,
		TakenAt:      time.Now().Add(-time.Minute).UTC().Truncate(time.Microsecond),
		BalanceCount: 3, Status: snapshot.StatusOK,
	}
	newer := snapshot.Checkpoint{
		ID: uuid.New(), Account: ref,
		TakenAt:      time.Now().UTC().Truncate(time.Microsecond),
		BalanceCount: 5, Status: snapshot.StatusFailed, Error: "partial venue outage",
	}
	for _, c := range []snapshot.Checkpoint{older, newer} {
		if err := store.RecordSnapshot(ctx, c); err != nil {
			t.Fatalf("RecordSnapshot: %v", err)
		}
	}

	got, err := store.LastSnapshot(ctx, ref)
	if err != nil {
		t.Fatalf("LastSnapshot: %v", err)
	}
	if got.ID != newer.ID || !got.TakenAt.Equal(newer.TakenAt) ||
		got.Status != snapshot.StatusFailed || got.Error != newer.Error || got.BalanceCount != 5 {
		t.Errorf("LastSnapshot mismatch:\n got %+v\nwant %+v", got, newer)
	}

	// Other accounts remain isolated.
	if _, err := store.LastSnapshot(ctx, account.Ref{Venue: "bybit", Type: account.TypeMargin}); !errors.Is(err, ports.ErrNotFound) {
		t.Errorf("expected ErrNotFound for other account, got %v", err)
	}

	// Migrations are idempotent: a second Connect must not fail.
	pool2, err := Connect(ctx, config.Postgres{DSN: pool.Config().ConnString()})
	if err != nil {
		t.Fatalf("second Connect (idempotent migrations): %v", err)
	}
	pool2.Close()
}
