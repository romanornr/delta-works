// Package outbox relays rows from the transactional outbox to the event
// bus (ADR-0008). Stores write outbox rows in their own transactions; this
// relay is the only path from those rows onto the bus, giving
// at-least-once delivery into the bus while Postgres stays the truth.
package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jonboulle/clockwork"

	"github.com/romanornr/delta-works/internal/bus"
	"github.com/romanornr/delta-works/internal/events"
	"github.com/romanornr/delta-works/internal/log"
	"github.com/romanornr/delta-works/internal/ports"
)

// retention is how long published rows are kept for debugging before the
// periodic cleanup deletes them.
const retention = 7 * 24 * time.Hour

// cleanupInterval spaces the retention deletes; retention is days, so
// hourly is more than fine-grained enough.
const cleanupInterval = time.Hour

// Service is the single relay goroutine: poll, publish, mark, repeat.
type Service struct {
	store    ports.OutboxStore
	bus      bus.Bus
	clk      clockwork.Clock
	log      log.Logger
	interval time.Duration
	batch    int
	metrics  *Metrics
}

// New builds the relay. Metrics must not be nil.
func New(
	store ports.OutboxStore,
	eventBus bus.Bus,
	clk clockwork.Clock,
	logger log.Logger,
	interval time.Duration,
	batch int,
	metrics *Metrics,
) *Service {
	return &Service{
		store:    store,
		bus:      eventBus,
		clk:      clk,
		log:      log.Component(logger, "outbox"),
		interval: interval,
		batch:    batch,
		metrics:  metrics,
	}
}

// Run blocks until ctx is canceled. A store failure is infrastructure
// loss and stops the service so the process can fail fast and restart;
// unpublished rows survive in Postgres and drain after the restart.
func (s *Service) Run(ctx context.Context) error {
	ticker := s.clk.NewTicker(s.interval)
	defer ticker.Stop()
	cleanup := s.clk.NewTicker(cleanupInterval)
	defer cleanup.Stop()

	// First drain immediately so rows written while the process was down
	// do not wait another interval.
	if err := s.drain(ctx); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.Chan():
			if err := s.drain(ctx); err != nil {
				return err
			}
		case <-cleanup.Chan():
			if err := s.cleanup(ctx); err != nil {
				return err
			}
		}
	}
}

// drain publishes batches until the backlog is smaller than one batch, so
// a burst clears at publish speed instead of one batch per poll interval.
func (s *Service) drain(ctx context.Context) error {
	for {
		n, err := s.store.PublishPending(ctx, s.batch, func(m events.OutboxMessage) error {
			return s.bus.Publish(ctx, bus.Event{
				Subject: m.Subject,
				At:      m.CreatedAt,
				Payload: json.RawMessage(m.Payload),
			})
		})
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("outbox store: %w", err)
		}
		s.metrics.observePublished(n)
		if n < s.batch {
			break
		}
	}
	return s.observeBacklog(ctx)
}

func (s *Service) observeBacklog(ctx context.Context) error {
	rows, oldest, err := s.store.UnpublishedStats(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("outbox store: %w", err)
	}
	var age time.Duration
	if rows > 0 {
		// The row timestamp comes from the database clock; clamp so skew
		// against the process clock cannot report a negative age.
		age = max(0, s.clk.Now().Sub(oldest))
	}
	s.metrics.observeBacklog(rows, age)
	return nil
}

func (s *Service) cleanup(ctx context.Context) error {
	n, err := s.store.DeletePublishedBefore(ctx, s.clk.Now().Add(-retention))
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("outbox store: %w", err)
	}
	if n > 0 {
		s.log.Debug().Int64("rows", n).Msg("published outbox rows deleted")
	}
	return nil
}
