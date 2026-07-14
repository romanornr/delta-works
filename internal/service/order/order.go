// Package order orchestrates order placement, cancellation and venue event
// application per the order state machine (docs/specs/manual-trading.md). The pending
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

var (
	// ErrSubmitUnsettled reports that the venue submit did not conclusively
	// succeed or fail within the retry budget. The order stays pending;
	// reconciliation adopts it or marks it rejected after the grace window.
	ErrSubmitUnsettled = errors.New("submit unsettled; order stays pending until reconciliation")
	// ErrIdentityMismatch reports reuse of a client order ID with different immutable fields.
	ErrIdentityMismatch = errors.New("client order ID identity mismatch")
	// ErrTerminal reports an attempt to cancel an order already in a terminal state.
	ErrTerminal = errors.New("order is terminal")
	// ErrVenueNotConfigured reports a request for a venue without trading enabled.
	ErrVenueNotConfigured = errors.New("venue not configured for trading")
)

// PlaceResult is the locally persisted state after a placement attempt.
type PlaceResult struct {
	ClientOrderID domain.ClientOrderID
	Status        domain.Status
}

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
func (s *Service) Place(ctx context.Context, req domain.Request) (PlaceResult, error) {
	req, venue, existing, err := s.preparePlace(ctx, req)
	if err != nil {
		return PlaceResult{}, err
	}
	if existing != nil {
		return *existing, nil
	}

	ack, err := backoff.Retry(ctx, func() (domain.Ack, error) {
		a, err := venue.Placer.PlaceOrder(ctx, req)
		if errors.Is(err, ports.ErrAuth) || errors.Is(err, ports.ErrTradingUnsupported) {
			return domain.Ack{}, backoff.Permanent(err)
		}
		return a, err
	}, backoff.WithMaxElapsedTime(s.submitBudget))
	if err != nil {
		return s.submitFailure(ctx, req, err)
	}

	// The ack carries no cumulative fill quantity, so a zero here is
	// harmless: fill-bearing events outrank or out-fill it and stale acks
	// drop by rank (see the state machine's guards).
	if err := s.apply(ctx, domain.SourceAck, domain.Event{
		Ref:    ack.Ref,
		Status: ack.Status,
		At:     ack.AcceptedAt,
	}); err != nil {
		return s.acceptedUnsettled(req, err)
	}
	stored, err := s.store.GetOrder(ctx, req.ClientOrderID)
	if err != nil {
		return s.acceptedUnsettled(req, err)
	}
	return placeResult(stored), nil
}

// acceptedUnsettled reports a submit the venue accepted whose local state
// could not be updated or read back. The caller still gets the client
// order ID: the order is live at the venue and reconciliation converges
// the local row.
func (s *Service) acceptedUnsettled(req domain.Request, err error) (PlaceResult, error) {
	s.log.Error().Str("venue", string(req.Instrument.Venue)).
		Str("client_order_id", string(req.ClientOrderID)).Err(err).
		Msg("venue accepted the submit but the local update failed")
	return PlaceResult{ClientOrderID: req.ClientOrderID, Status: domain.StatusPending},
		fmt.Errorf("%w: %w", ErrSubmitUnsettled, err)
}

func (s *Service) preparePlace(ctx context.Context, req domain.Request) (domain.Request, Venue, *PlaceResult, error) {
	if req.ClientOrderID == "" {
		req.ClientOrderID = domain.ClientOrderID(id.New())
	}
	if req.BotID == "" {
		req.BotID = "manual"
	}
	// A retry for an order that already advanced needs no venue, so the
	// venue is only required when inserting or resubmitting. That keeps
	// supplied-ID retries idempotent even after a venue is deconfigured.
	venue, configured := s.venues[req.Instrument.Venue]
	if configured {
		inserted, err := s.store.CreatePending(ctx, req)
		if err != nil {
			return req, Venue{}, nil, err
		}
		if inserted {
			return req, venue, nil, nil
		}
	}
	stored, err := s.store.GetOrder(ctx, req.ClientOrderID)
	if err != nil {
		if !configured && errors.Is(err, ports.ErrNotFound) {
			return req, Venue{}, nil, fmt.Errorf("%w: %q", ErrVenueNotConfigured, req.Instrument.Venue)
		}
		return req, Venue{}, nil, err
	}
	if !sameIdentity(stored, req) {
		return req, Venue{}, nil, fmt.Errorf("%w: %s", ErrIdentityMismatch, req.ClientOrderID)
	}
	if stored.VenueOrderID == "" && stored.Status == domain.StatusPending {
		if !configured {
			return req, Venue{}, nil, fmt.Errorf("%w: %q", ErrVenueNotConfigured, req.Instrument.Venue)
		}
		return req, venue, nil, nil
	}
	result := placeResult(stored)
	return req, Venue{}, &result, nil
}

func (s *Service) submitFailure(ctx context.Context, req domain.Request, err error) (PlaceResult, error) {
	// Recovery reads and writes must survive the caller's context: a
	// canceled deadline is often exactly why we are here.
	ctx = context.WithoutCancel(ctx)
	if errors.Is(err, ports.ErrAuth) || errors.Is(err, ports.ErrTradingUnsupported) {
		// No venue order can exist, and the same failure blocks the venue
		// lookups reconciliation would need, so settle the row now.
		if applyErr := s.apply(ctx, domain.SourceLocal, domain.Event{
			Ref:    domain.Ref{Instrument: req.Instrument, ClientOrderID: req.ClientOrderID},
			Status: domain.StatusRejected,
			Reason: "submit failed: " + err.Error(),
			At:     s.clk.Now(),
		}); applyErr != nil {
			s.log.Error().Str("client_order_id", string(req.ClientOrderID)).
				Err(applyErr).Msg("could not reject order after permanent submit failure")
		}
		return PlaceResult{ClientOrderID: req.ClientOrderID, Status: domain.StatusRejected}, err
	}
	// Everything else is ambiguous: the venue may hold the order. That
	// covers duplicate-ID venue errors (GCT gives no way to classify
	// them) and context cancellation that raced an in-flight submit.
	// Reconciliation adopts the pending order by client order ID or
	// rejects it after the grace window.
	s.log.Error().Str("venue", string(req.Instrument.Venue)).
		Str("client_order_id", string(req.ClientOrderID)).Err(err).
		Msg("submit unsettled after retries")
	stored, getErr := s.store.GetOrder(ctx, req.ClientOrderID)
	if getErr != nil {
		return PlaceResult{ClientOrderID: req.ClientOrderID, Status: domain.StatusPending},
			fmt.Errorf("%w: %w", ErrSubmitUnsettled, err)
	}
	return placeResult(stored), fmt.Errorf("%w: %w", ErrSubmitUnsettled, err)
}

func sameIdentity(stored ports.StoredOrder, req domain.Request) bool {
	return stored.ClientOrderID == req.ClientOrderID &&
		stored.BotID == req.BotID &&
		stored.Instrument.Venue == req.Instrument.Venue &&
		stored.Instrument.Base == req.Instrument.Base &&
		stored.Instrument.Quote == req.Instrument.Quote &&
		stored.Side == req.Side && stored.Type == req.Type &&
		stored.Price.Equal(req.Price) && stored.Qty.Equal(req.Qty)
}

func placeResult(stored ports.StoredOrder) PlaceResult {
	return PlaceResult{ClientOrderID: stored.ClientOrderID, Status: stored.Status}
}

// Cancel records the cancel intent and asks the venue. The canceled state
// itself arrives later like any other venue event.
func (s *Service) Cancel(ctx context.Context, orderID domain.ClientOrderID) (domain.Status, error) {
	stored, err := s.store.GetOrder(ctx, orderID)
	if err != nil {
		return "", err
	}
	if stored.Status.Terminal() {
		return "", fmt.Errorf("%w: %s is already %s", ErrTerminal, orderID, stored.Status)
	}
	venue, ok := s.venues[stored.Instrument.Venue]
	if !ok {
		return "", fmt.Errorf("%w: %q", ErrVenueNotConfigured, stored.Instrument.Venue)
	}
	if err := s.store.MarkCancelRequested(ctx, orderID, s.clk.Now()); err != nil {
		return "", err
	}
	if err := venue.Placer.CancelOrder(ctx, domain.Ref{
		Instrument:    stored.Instrument,
		ClientOrderID: stored.ClientOrderID,
		VenueOrderID:  stored.VenueOrderID,
	}); err != nil {
		return "", err
	}
	return stored.Status, nil
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
	decision, note, err := s.store.ApplyEvent(ctx, source, ev)
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
	if note.UnmatchedQty.IsPositive() {
		s.metrics.observeUnmatched(venue)
	}
	if note.FillConflict {
		s.metrics.observeDropped(venue, "fill_id_conflict")
		s.log.Warn().Str("venue", string(venue)).
			Str("client_order_id", string(ev.Ref.ClientOrderID)).
			Str("venue_fill_id", ev.VenueFillID).
			Msg("cumulative fill advanced under an already-recorded venue fill ID; ledger did not post the delta")
	}
	if decision.Drop != "" && !decision.Transition && !decision.FillDelta.IsPositive() {
		s.metrics.observeDropped(venue, string(decision.Drop))
	}
	if decision.FillAnomaly {
		s.metrics.observeDropped(venue, "negative_fill_delta")
	}
	return nil
}
