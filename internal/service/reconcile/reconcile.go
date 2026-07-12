// Package reconcile converges venue and local order state according to the
// reconciliation rules in docs/specs/m2-oms.md.
package reconcile

import (
	"context"
	"errors"
	"fmt"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/romanornr/delta-works/internal/bus"
	"github.com/romanornr/delta-works/internal/clock"
	"github.com/romanornr/delta-works/internal/domain/instrument"
	domain "github.com/romanornr/delta-works/internal/domain/order"
	"github.com/romanornr/delta-works/internal/log"
	"github.com/romanornr/delta-works/internal/ports"
	orderservice "github.com/romanornr/delta-works/internal/service/order"
)

// SubjectOrphan is published when a venue reports an open order we do not
// know. Orphans are never adopted (docs/specs/m2-oms.md).
const SubjectOrphan = "reconcile.orphan"

// OrphanPayload is the bus payload for SubjectOrphan.
type OrphanPayload struct {
	Venue         instrument.VenueID
	VenueOrderID  string
	ClientOrderID domain.ClientOrderID
	Base, Quote   string
}

// Venue is one venue the loop reconciles.
type Venue struct {
	ID     instrument.VenueID
	Placer ports.OrderPlacer
}

type venueLoop struct {
	Venue
	kick        chan struct{}
	lastOrphans map[string]struct{}
}

// Service reconciles active local orders against venue order state.
type Service struct {
	venues   []*venueLoop
	store    ports.OrderStore
	bus      bus.Bus
	clk      clock.Clock
	log      log.Logger
	interval time.Duration
	metrics  *Metrics
}

// New builds the reconciliation service. Metrics must not be nil.
func New(
	venues []Venue,
	store ports.OrderStore,
	b bus.Bus,
	clk clock.Clock,
	logger log.Logger,
	interval time.Duration,
	metrics *Metrics,
) *Service {
	loops := make([]*venueLoop, 0, len(venues))
	for _, v := range venues {
		loops = append(loops, &venueLoop{
			Venue:       v,
			kick:        make(chan struct{}, 1),
			lastOrphans: make(map[string]struct{}),
		})
	}
	return &Service{
		venues: loops, store: store, bus: b, clk: clk,
		log: log.Component(logger, "reconcile"), interval: interval, metrics: metrics,
	}
}

// Run reconciles each venue until ctx is canceled. Venue failures skip one
// pass; store failures stop the service so the process can fail fast.
func (s *Service) Run(ctx context.Context) error {
	unsubscribe, err := s.bus.Subscribe(orderservice.SubjectStreamReconnected, s.handleReconnect)
	if err != nil {
		return fmt.Errorf("reconcile: subscribe to stream reconnects: %w", err)
	}
	defer unsubscribe()

	g, ctx := errgroup.WithContext(ctx)
	for _, v := range s.venues {
		g.Go(func() error { return s.runVenue(ctx, v) })
	}
	return g.Wait()
}

func (s *Service) handleReconnect(_ context.Context, event bus.Event) {
	venue, ok := event.Payload.(instrument.VenueID)
	if !ok {
		s.log.Warn().Str("payload_type", fmt.Sprintf("%T", event.Payload)).
			Msg("stream reconnect payload has wrong type")
		return
	}
	for _, v := range s.venues {
		if v.ID != venue {
			continue
		}
		select {
		case v.kick <- struct{}{}:
		default:
		}
	}
}

func (s *Service) runVenue(ctx context.Context, v *venueLoop) error {
	if err := s.pass(ctx, v); err != nil {
		return passError(ctx, err)
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-s.clk.After(s.interval):
		case <-v.kick:
		}
		if err := s.pass(ctx, v); err != nil {
			return passError(ctx, err)
		}
	}
}

func passError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return nil
	}
	return err
}

func (s *Service) pass(ctx context.Context, v *venueLoop) error {
	start := s.clk.Now()
	venueOrders, err := v.Placer.OpenOrders(ctx)
	if err != nil {
		s.log.Error().Str("venue", string(v.ID)).Err(err).Msg("open orders failed")
		return nil
	}
	local, err := s.store.ListActiveOrders(ctx, v.ID)
	if err != nil {
		return fmt.Errorf("reconcile store: list active orders: %w", err)
	}

	venueByClient := make(map[domain.ClientOrderID]domain.Snapshot, len(venueOrders))
	for _, snap := range venueOrders {
		if snap.Ref.ClientOrderID != "" {
			venueByClient[snap.Ref.ClientOrderID] = snap
		}
	}
	localActive := make(map[domain.ClientOrderID]struct{}, len(local))
	for _, lo := range local {
		localActive[lo.ClientOrderID] = struct{}{}
		if err := s.reconcileLocal(ctx, v.Venue, lo, venueByClient); err != nil {
			return err
		}
	}

	orphans, currentOrphans, err := s.reconcileOrphans(ctx, v, venueOrders, localActive)
	if err != nil {
		return err
	}
	v.lastOrphans = currentOrphans
	finished := s.clk.Now()
	s.metrics.observeSuccess(v.ID, finished.Sub(start), finished, orphans)
	return nil
}

func (s *Service) reconcileLocal(
	ctx context.Context,
	v Venue,
	lo ports.StoredOrder,
	venueByClient map[domain.ClientOrderID]domain.Snapshot,
) error {
	if snap, ok := venueByClient[lo.ClientOrderID]; ok {
		adoptVenueOrderID := lo.VenueOrderID == "" && snap.Ref.VenueOrderID != ""
		if snap.Status == lo.Status && snap.FilledQty.Equal(lo.FilledQty) && !adoptVenueOrderID {
			return nil
		}
		kind := "drift"
		if lo.Status == domain.StatusPending || adoptVenueOrderID {
			kind = "adopted"
		}
		return s.applyAndCount(ctx, v.ID, snap, kind, "")
	}
	if lo.Status == domain.StatusPending {
		return s.reconcilePending(ctx, v, lo)
	}
	return s.reconcileMissingActive(ctx, v, lo)
}

func (s *Service) reconcilePending(ctx context.Context, v Venue, lo ports.StoredOrder) error {
	if s.clk.Now().Sub(lo.CreatedAt) < 2*s.interval {
		return nil
	}
	snap, err := v.Placer.GetOrder(ctx, storedRef(lo))
	switch {
	case errors.Is(err, ports.ErrNotFound):
		return s.applyAndCount(ctx, v.ID, domain.Snapshot{
			Ref: storedRef(lo), Status: domain.StatusRejected,
			FilledQty: lo.FilledQty, UpdatedAt: s.clk.Now(),
		}, "submit_lost", "submit-lost")
	case err != nil:
		s.log.Warn().Str("venue", string(v.ID)).
			Str("client_order_id", string(lo.ClientOrderID)).Err(err).
			Msg("pending order lookup failed")
		return nil
	default:
		return s.applyAndCount(ctx, v.ID, withLocalRef(snap, lo), "adopted", "")
	}
}

func (s *Service) reconcileMissingActive(ctx context.Context, v Venue, lo ports.StoredOrder) error {
	snap, err := v.Placer.GetOrder(ctx, storedRef(lo))
	if err != nil {
		s.log.Error().Str("venue", string(v.ID)).
			Str("client_order_id", string(lo.ClientOrderID)).Err(err).
			Msg("active order lookup failed")
		return nil
	}
	return s.applyAndCount(ctx, v.ID, withLocalRef(snap, lo), "closed", "")
}

// withLocalRef restores our identity keys on a point-lookup snapshot; some
// venues omit the client order ID in single-order responses, and applying
// an event with an empty key would miss the row it was fetched for.
func withLocalRef(snap domain.Snapshot, lo ports.StoredOrder) domain.Snapshot {
	snap.Ref.ClientOrderID = lo.ClientOrderID
	if snap.Ref.VenueOrderID == "" {
		snap.Ref.VenueOrderID = lo.VenueOrderID
	}
	return snap
}

func (s *Service) reconcileOrphans(
	ctx context.Context,
	v *venueLoop,
	venueOrders []domain.Snapshot,
	localActive map[domain.ClientOrderID]struct{},
) (int, map[string]struct{}, error) {
	current := make(map[string]struct{})
	count := 0
	for _, snap := range venueOrders {
		clientID := snap.Ref.ClientOrderID
		if _, ok := localActive[clientID]; clientID != "" && ok {
			continue
		}
		orphan, err := s.isOrphan(ctx, v.ID, snap)
		if err != nil {
			return 0, nil, err
		}
		if !orphan {
			continue
		}
		count++
		s.metrics.observeDiff(v.ID, "orphan")
		current[snap.Ref.VenueOrderID] = struct{}{}
		if _, reported := v.lastOrphans[snap.Ref.VenueOrderID]; reported {
			continue
		}
		if err := s.publishOrphan(ctx, v.ID, snap); err != nil {
			return 0, nil, err
		}
	}
	return count, current, nil
}

func (s *Service) isOrphan(ctx context.Context, venue instrument.VenueID, snap domain.Snapshot) (bool, error) {
	if snap.Ref.ClientOrderID == "" {
		return true, nil
	}
	_, err := s.store.GetOrder(ctx, snap.Ref.ClientOrderID)
	switch {
	case err == nil:
		s.log.Warn().Str("venue", string(venue)).
			Str("client_order_id", string(snap.Ref.ClientOrderID)).
			Str("venue_order_id", snap.Ref.VenueOrderID).
			Msg("terminal local order is open on venue")
		s.metrics.observeDiff(venue, "terminal_open")
		return false, nil
	case errors.Is(err, ports.ErrNotFound):
		return true, nil
	default:
		return false, fmt.Errorf("reconcile store: get order: %w", err)
	}
}

func (s *Service) publishOrphan(ctx context.Context, venue instrument.VenueID, snap domain.Snapshot) error {
	err := s.bus.Publish(ctx, bus.Event{
		Subject: SubjectOrphan,
		At:      s.clk.Now(),
		Payload: OrphanPayload{
			Venue: venue, VenueOrderID: snap.Ref.VenueOrderID,
			ClientOrderID: snap.Ref.ClientOrderID,
			Base:          string(snap.Ref.Instrument.Base), Quote: string(snap.Ref.Instrument.Quote),
		},
	})
	if err != nil {
		return fmt.Errorf("reconcile: publish orphan: %w", err)
	}
	return nil
}

func (s *Service) applyAndCount(
	ctx context.Context,
	venue instrument.VenueID,
	snap domain.Snapshot,
	kind, reason string,
) error {
	if err := s.apply(ctx, snap, reason); err != nil {
		return err
	}
	s.metrics.observeDiff(venue, kind)
	return nil
}

func (s *Service) apply(ctx context.Context, snap domain.Snapshot, reason string) error {
	ev := domain.Event{
		Ref: snap.Ref, Status: snap.Status, FilledQty: snap.FilledQty,
		FillPrice: snap.AvgFillPrice, Reason: reason, At: snap.UpdatedAt,
	}
	if ev.At.IsZero() {
		ev.At = s.clk.Now()
	}
	decision, err := s.store.ApplyEvent(ctx, domain.SourceReconcile, ev)
	switch {
	case errors.Is(err, ports.ErrNotFound):
		s.log.Warn().Str("venue", string(ev.Ref.Instrument.Venue)).
			Str("client_order_id", string(ev.Ref.ClientOrderID)).
			Msg("order disappeared during reconciliation")
		return nil
	case err != nil:
		return fmt.Errorf("reconcile store: apply event: %w", err)
	}
	if decision.Drop != "" {
		s.log.Debug().Str("venue", string(ev.Ref.Instrument.Venue)).
			Str("client_order_id", string(ev.Ref.ClientOrderID)).
			Str("reason", string(decision.Drop)).Msg("reconcile event dropped")
	}
	return nil
}

func storedRef(lo ports.StoredOrder) domain.Ref {
	return domain.Ref{
		Instrument: lo.Instrument, ClientOrderID: lo.ClientOrderID,
		VenueOrderID: lo.VenueOrderID,
	}
}
