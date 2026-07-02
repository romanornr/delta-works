package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog"
	"go.uber.org/fx"

	"github.com/romanornr/delta-works/internal/config"
	"github.com/romanornr/delta-works/internal/domain/portfolio"
	"github.com/romanornr/delta-works/internal/domain/transfer"
	"github.com/romanornr/delta-works/internal/observability"
	"github.com/romanornr/delta-works/internal/storage"
)

// memStore is a tiny in-memory implementation of storage.Store.
// It exists only for learning/probing interface call flows.
type memStore struct {
	log       zerolog.Logger
	snapshots *memSnapshotStore
	transfers *memTransferStore
}

func newMemStore(log zerolog.Logger) storage.Store {
	base := log.With().Str("component", "devprobe-storage").Logger()
	return &memStore{
		log:       base,
		snapshots: &memSnapshotStore{log: base},
		transfers: &memTransferStore{log: base},
	}
}

func (s *memStore) Snapshots() storage.SnapshotStore { return s.snapshots }
func (s *memStore) Transfers() storage.TransferStore { return s.transfers }

func (s *memStore) Close(ctx context.Context) error {
	s.log.Info().Msg("Close called")
	return nil
}

func (s *memStore) Ping(ctx context.Context) error {
	s.log.Info().Msg("Ping called")
	return nil
}

type memSnapshotStore struct {
	log zerolog.Logger
}

func (s *memSnapshotStore) Write(ctx context.Context, snap portfolio.Snapshot) error {
	s.log.Info().Str("exchange", snap.Exchange).Str("account", snap.Account.String()).Time("captured_at", snap.CapturedAt).Msg("SnapshotStore.Write called")
	return nil
}

func (s *memSnapshotStore) Latest(ctx context.Context, exchange string, account portfolio.AccountType) (*portfolio.Snapshot, error) {
	s.log.Info().Str("exchange", exchange).Str("account", account.String()).Msg("SnapshotStore.Latest called")
	return nil, nil
}

func (s *memSnapshotStore) Range(ctx context.Context, exchange string, account portfolio.AccountType, from, to time.Time) ([]portfolio.Snapshot, error) {
	s.log.Info().Str("exchange", exchange).Str("account", account.String()).Time("from", from).Time("to", to).Msg("SnapshotStore.Range called")
	return nil, nil
}

type memTransferStore struct {
	log zerolog.Logger
}

func (s *memTransferStore) Write(ctx context.Context, t transfer.Transfer) error {
	s.log.Info().Str("exchange", t.Exchange).Str("direction", string(t.Direction)).Str("asset", t.Asset).Time("completed_at", t.CompletedAt).Msg("TransferStore.Write called")
	return nil
}

func (s *memTransferStore) WriteBatch(ctx context.Context, batch []transfer.Transfer) error {
	s.log.Info().Int("count", len(batch)).Msg("TransferStore.WriteBatch called")
	for _, t := range batch {
		if err := s.Write(ctx, t); err != nil {
			return err
		}
	}
	return nil
}

func (s *memTransferStore) LastTime(ctx context.Context, exchange string, direction transfer.Direction) (time.Time, error) {
	s.log.Info().Str("exchange", exchange).Str("direction", string(direction)).Msg("TransferStore.LastTime called")
	return time.Time{}, nil
}

func (s *memTransferStore) Range(ctx context.Context, exchange string, from, to time.Time) ([]transfer.Transfer, error) {
	s.log.Info().Str("exchange", exchange).Time("from", from).Time("to", to).Msg("TransferStore.Range called")
	return nil, nil
}

func main() {
	var store storage.Store
	var log zerolog.Logger

	app := fx.New(config.Module, observability.Module, fx.Provide(newMemStore), fx.Populate(&store, &log))

	startCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := app.Start(startCtx); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to start app: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_ = store.Ping(ctx)
	_, _ = store.Snapshots().Latest(ctx, "bybit", portfolio.AccountSpot)
	_, _ = store.Transfers().LastTime(ctx, "bybit", transfer.Inbound)
	_ = store.Close(ctx)

	StopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := app.Stop(StopCtx); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to stop app: %v\n", err)
		os.Exit(1)
	}

	log.Info().Msg("devprobe-storage ok")

}
