// Package snapshot polls venue balances on an interval and persists them:
// time-series rows to the series writer first, then a checkpoint row, so a
// checkpoint always means the data reached the time-series store.
package snapshot

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"github.com/romanornr/delta-works/internal/bus"
	"github.com/romanornr/delta-works/internal/clock"
	"github.com/romanornr/delta-works/internal/domain/account"
	"github.com/romanornr/delta-works/internal/domain/instrument"
	"github.com/romanornr/delta-works/internal/exchange"
	"github.com/romanornr/delta-works/internal/log"
	"github.com/romanornr/delta-works/internal/ports"
)

// SubjectTaken is published after a snapshot is durably checkpointed.
const SubjectTaken = "snapshot.taken"

// recordTimeout bounds the checkpoint write that runs on a detached context
// after a successful series flush.
const recordTimeout = 5 * time.Second

// Target is one (venue, account) pair to poll.
type Target struct {
	Venue   instrument.VenueID
	Account account.Type
}

// Service polls all targets concurrently, one goroutine per target.
type Service struct {
	registry    exchange.Registry
	series      ports.SeriesWriter
	checkpoints ports.CheckpointStore
	bus         bus.Bus
	clk         clock.Clock
	log         log.Logger
	interval    time.Duration
	targets     []Target
	metrics     *Metrics
	writeMu     sync.Mutex
}

// New builds the service. Metrics must not be nil.
func New(
	registry exchange.Registry,
	series ports.SeriesWriter,
	checkpoints ports.CheckpointStore,
	eventBus bus.Bus,
	clk clock.Clock,
	logger log.Logger,
	interval time.Duration,
	targets []Target,
	metrics *Metrics,
) *Service {
	return &Service{
		registry:    registry,
		series:      series,
		checkpoints: checkpoints,
		bus:         eventBus,
		clk:         clk,
		log:         log.Component(logger, "snapshot"),
		interval:    interval,
		targets:     targets,
		metrics:     metrics,
	}
}

// Run blocks until ctx is canceled or an infrastructure failure occurs.
// Venue failures never stop the service: they are logged, counted, and left
// to the circuit breaker and the next tick. A checkpoint store failure is
// treated as infrastructure loss and stops the whole service so the process
// can fail fast and restart.
func (s *Service) Run(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)
	for _, t := range s.targets {
		g.Go(func() error { return s.pollLoop(ctx, t) })
	}
	return g.Wait()
}

func (s *Service) pollLoop(ctx context.Context, t Target) error {
	ticker := s.clk.NewTicker(s.interval)
	defer ticker.Stop()

	// First snapshot immediately so a fresh process is observable without
	// waiting a full interval.
	if err := s.snapshot(ctx, t); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C():
			if err := s.snapshot(ctx, t); err != nil {
				return err
			}
		}
	}
}

// snapshot returns a non-nil error only for infrastructure failures.
func (s *Service) snapshot(ctx context.Context, t Target) error {
	ex, err := s.registry.Get(t.Venue)
	if err != nil {
		return err // a missing venue is a wiring bug, not a venue outage
	}

	start := s.clk.Now()
	// The fetch gets its own deadline so a stalled venue call cannot eat
	// the next tick or hold up shutdown; recording and publishing below
	// stay on the parent context.
	fetchCtx, cancel := context.WithTimeout(ctx, s.interval/2)
	balances, fetchErr := backoff.Retry(fetchCtx, func() ([]account.Balance, error) {
		bs, err := ex.Balances(fetchCtx, t.Account)
		if errors.Is(err, ports.ErrAuth) || errors.Is(err, ports.ErrUnsupportedAccount) {
			return nil, backoff.Permanent(err)
		}
		return bs, err
	}, backoff.WithMaxElapsedTime(s.interval/2))
	cancel()
	takenAt := s.clk.Now()

	if ctx.Err() != nil {
		return nil
	}

	checkpoint := ports.SnapshotCheckpoint{
		ID:      uuid.New(),
		Account: account.Ref{Venue: t.Venue, Type: t.Account},
		TakenAt: takenAt,
	}

	if fetchErr != nil {
		s.metrics.observeError(t)
		s.log.Error().Str("venue", string(t.Venue)).Str("account", string(t.Account)).
			Err(fetchErr).Msg("balance fetch failed")
		checkpoint.Status = ports.CheckpointFailed
		checkpoint.Error = fetchErr.Error()
		return s.record(ctx, checkpoint)
	}

	snap := account.Snapshot{Account: checkpoint.Account, TakenAt: takenAt, Balances: balances}
	if err := s.writeSeries(ctx, snap); err != nil {
		s.metrics.observeError(t)
		s.log.Error().Str("venue", string(t.Venue)).Err(err).Msg("series write failed")
		checkpoint.Status = ports.CheckpointFailed
		checkpoint.Error = err.Error()
		return s.record(ctx, checkpoint)
	}

	// The rows are durable in the series store now. Shutdown must not cancel
	// the checkpoint that anchors them, or gap detection would report a gap
	// where data exists, so the write gets a detached bounded context.
	recordCtx, recordCancel := context.WithTimeout(context.WithoutCancel(ctx), recordTimeout)
	defer recordCancel()
	checkpoint.Status = ports.CheckpointOK
	checkpoint.BalanceCount = len(snap.NonZero())
	if err := s.record(recordCtx, checkpoint); err != nil {
		return err
	}

	s.metrics.observeSuccess(t, s.clk.Now().Sub(start), takenAt)
	s.log.Debug().Str("venue", string(t.Venue)).Str("account", string(t.Account)).
		Int("balances", checkpoint.BalanceCount).Msg("snapshot taken")
	return s.bus.Publish(ctx, bus.Event{Subject: SubjectTaken, At: takenAt, Payload: snap})
}

// writeSeries holds one lock across write and flush. All targets share the
// series writer, and the writer only serializes individual calls; without
// this lock one target's flush could carry another target's rows, so a
// checkpoint would no longer describe its own snapshot's durability.
func (s *Service) writeSeries(ctx context.Context, snap account.Snapshot) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := s.series.WriteBalanceSnapshot(ctx, snap); err != nil {
		return err
	}
	return s.series.Flush(ctx)
}

func (s *Service) record(ctx context.Context, c ports.SnapshotCheckpoint) error {
	if err := s.checkpoints.RecordSnapshot(ctx, c); err != nil && ctx.Err() == nil {
		return fmt.Errorf("checkpoint store: %w", err)
	}
	return nil
}
