// Package order orchestrates order placement, cancellation and venue event
// application per the M2 state machine (docs/specs/m2-oms.md). The pending
// row is persisted before the venue submit; venue events reach Postgres
// through OrderStore.ApplyEvent, which also feeds the outbox (ADR-0008).
package order

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cenkalti/backoff/v5"
	"golang.org/x/sync/errgroup"

	"github.com/romanornr/delta-works/internal/clock"
	"github.com/romanornr/delta-works/internal/domain/instrument"
	domain "github.com/romanornr/delta-works/internal/domain/order"
	"github.com/romanornr/delta-works/internal/id"
	"github.com/romanornr/delta-works/internal/log"
	"github.com/romanornr/delta-works/internal/ports"
)

// SubjectStreamReconnected is published (by the streamer wiring, payload:
// the venue ID) after a private stream reconnects, so reconciliation can
// close the event gap immediately.
const SubjectStreamReconnected = "stream.reconnected"

// streamRetryDelay spaces attempts to re-establish a venue stream after it
// failed to start or ended. The adapter owns socket-level reconnects; this
// only covers the stream being torn down entirely.
const streamRetryDelay = 30 * time.Second

// ErrSubmitUnsettled reports that the venue submit did not conclusively
// succeed or fail within the retry budget. The order stays pending;
// reconciliation adopts it or marks it rejected after the grace window.
var ErrSubmitUnsettled = errors.New("submit unsettled; order stays pending until reconciliation")

// Venue is one tradable venue: the resilience-wrapped order surface and
// its private event stream.
type Venue struct {
	ID       instrument.VenueID
	Placer   ports.OrderPlacer
	Streamer ports.PrivateStreamer
}

// Service is the only path through which orders are placed or canceled
// (ADR-0007: the control plane is the sole client surface).
type Service struct {
	venues       map[instrument.VenueID]Venue
	store        ports.OrderStore
	clk          clock.Clock
	log          log.Logger
	submitBudget time.Duration
	metrics      *Metrics
}

// New builds the service. Metrics must not be nil. submitBudget caps the
// total time spent retrying one venue submit with the same ULID.
func New(venues []Venue, store ports.OrderStore, clk clock.Clock, logger log.Logger, submitBudget time.Duration, metrics *Metrics) *Service {
	byID := make(map[instrument.VenueID]Venue, len(venues))
	for _, v := range venues {
		byID[v.ID] = v
	}
	return &Service{
		venues:       byID,
		store:        store,
		clk:          clk,
		log:          log.Component(logger, "order"),
		submitBudget: submitBudget,
		metrics:      metrics,
	}
}

// Place persists the order as pending, submits it to the venue retrying
// ambiguous failures with the SAME client order ID, and applies the ack.
// The returned ID is valid even when the error is ErrSubmitUnsettled: the
// order exists locally and reconciliation settles its fate.
func (s *Service) Place(ctx context.Context, req domain.Request) (domain.ClientOrderID, error) {
	venue, ok := s.venues[req.Instrument.Venue]
	if !ok {
		return "", fmt.Errorf("order: venue %q not configured for trading", req.Instrument.Venue)
	}
	if req.ClientOrderID == "" {
		req.ClientOrderID = domain.ClientOrderID(id.New())
	}
	if req.BotID == "" {
		req.BotID = "manual"
	}

	if err := s.store.CreatePending(ctx, req); err != nil {
		return "", err
	}

	ack, err := backoff.Retry(ctx, func() (domain.Ack, error) {
		a, err := venue.Placer.PlaceOrder(ctx, req)
		if errors.Is(err, ports.ErrAuth) || errors.Is(err, ports.ErrTradingUnsupported) {
			return domain.Ack{}, backoff.Permanent(err)
		}
		return a, err
	}, backoff.WithMaxElapsedTime(s.submitBudget))
	if err != nil {
		// A duplicate-ID venue error also lands here: GCT gives no way to
		// classify it, so reconciliation adopting the pending order by
		// client order ID is the recovery path either way.
		s.log.Error().Str("venue", string(req.Instrument.Venue)).
			Str("client_order_id", string(req.ClientOrderID)).Err(err).
			Msg("submit unsettled after retries")
		return req.ClientOrderID, fmt.Errorf("%w: %w", ErrSubmitUnsettled, err)
	}

	// The ack carries no cumulative fill quantity, so a zero here is
	// harmless: fill-bearing events outrank or out-fill it and stale acks
	// drop by rank (see the state machine's guards).
	return req.ClientOrderID, s.apply(ctx, domain.SourceAck, domain.Event{
		Ref:    ack.Ref,
		Status: ack.Status,
		At:     ack.AcceptedAt,
	})
}

// Cancel records the cancel intent and asks the venue. The canceled state
// itself arrives later like any other venue event.
func (s *Service) Cancel(ctx context.Context, orderID domain.ClientOrderID) error {
	stored, err := s.store.GetOrder(ctx, orderID)
	if err != nil {
		return err
	}
	if stored.Status.Terminal() {
		return fmt.Errorf("order: %s is already %s", orderID, stored.Status)
	}
	venue, ok := s.venues[stored.Instrument.Venue]
	if !ok {
		return fmt.Errorf("order: venue %q not configured for trading", stored.Instrument.Venue)
	}
	if err := s.store.MarkCancelRequested(ctx, orderID, s.clk.Now()); err != nil {
		return err
	}
	return venue.Placer.CancelOrder(ctx, domain.Ref{
		Instrument:    stored.Instrument,
		ClientOrderID: stored.ClientOrderID,
		VenueOrderID:  stored.VenueOrderID,
	})
}

// Run consumes every venue's private stream until ctx is canceled. Stream
// setup failures and stream teardowns retry on a delay: they are venue
// trouble, not infrastructure loss. A store failure stops the service so
// the process fails fast (matching the snapshot service's policy).
func (s *Service) Run(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)
	for _, v := range s.venues {
		if v.Streamer == nil {
			// A venue can trade without a private stream; reconciliation
			// alone keeps it converged, just slower.
			s.log.Warn().Str("venue", string(v.ID)).Msg("no private stream configured")
			continue
		}
		g.Go(func() error { return s.consumeStream(ctx, v) })
	}
	return g.Wait()
}

func (s *Service) consumeStream(ctx context.Context, v Venue) error {
	for {
		events, err := v.Streamer.StreamOrderEvents(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			s.log.Error().Str("venue", string(v.ID)).Err(err).Msg("order stream failed to start")
			select {
			case <-ctx.Done():
				return nil
			case <-s.clk.After(streamRetryDelay):
				continue
			}
		}
		for ev := range events {
			if err := s.apply(ctx, domain.SourceStream, ev); err != nil {
				return err
			}
		}
		if ctx.Err() != nil {
			return nil
		}
		s.log.Warn().Str("venue", string(v.ID)).Msg("order stream ended; restarting")
	}
}

// apply feeds one venue event through the store. Events for unknown
// orders and events the machine drops are counted, never fatal; only a
// store failure propagates.
func (s *Service) apply(ctx context.Context, source domain.Source, ev domain.Event) error {
	if ev.At.IsZero() {
		// Receipt time is the best available occurred_at when the venue
		// reported none.
		ev.At = s.clk.Now()
	}
	venue := ev.Ref.Instrument.Venue
	decision, err := s.store.ApplyEvent(ctx, source, ev)
	switch {
	case errors.Is(err, ports.ErrNotFound):
		s.metrics.observeDropped(venue, "unknown_order")
		s.log.Warn().Str("venue", string(venue)).
			Str("client_order_id", string(ev.Ref.ClientOrderID)).
			Str("venue_order_id", ev.Ref.VenueOrderID).
			Msg("event for unknown order dropped")
		return nil
	case err != nil:
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("order store: %w", err)
	}
	if decision.Drop != "" && !decision.Transition && !decision.FillDelta.IsPositive() {
		s.metrics.observeDropped(venue, string(decision.Drop))
	}
	return nil
}
