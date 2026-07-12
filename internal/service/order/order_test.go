package order

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/shopspring/decimal"

	"github.com/romanornr/delta-works/internal/clock/clocktest"
	"github.com/romanornr/delta-works/internal/config"
	"github.com/romanornr/delta-works/internal/domain/instrument"
	domain "github.com/romanornr/delta-works/internal/domain/order"
	"github.com/romanornr/delta-works/internal/log"
	"github.com/romanornr/delta-works/internal/ports"
)

func testLogger(t *testing.T) log.Logger {
	t.Helper()
	logger, err := log.New(config.Log{Level: "error", Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	return logger
}

func testInstrument() instrument.Instrument {
	return instrument.Instrument{
		Venue: "bybit", Type: instrument.TypeSpot,
		Base: "BTC", Quote: "USDT", VenueSymbol: "BTCUSDT",
	}
}

type fakePlacer struct {
	mu       sync.Mutex
	submits  []domain.Request
	failures int
	err      error
	cancels  []domain.Ref
}

func (f *fakePlacer) PlaceOrder(_ context.Context, req domain.Request) (domain.Ack, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.submits = append(f.submits, req)
	if f.failures > 0 {
		f.failures--
		return domain.Ack{}, f.err
	}
	return domain.Ack{
		Ref:        domain.Ref{Instrument: req.Instrument, ClientOrderID: req.ClientOrderID, VenueOrderID: "v-1"},
		Status:     domain.StatusOpen,
		AcceptedAt: time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC),
	}, nil
}

func (f *fakePlacer) CancelOrder(_ context.Context, ref domain.Ref) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancels = append(f.cancels, ref)
	return f.err
}

func (f *fakePlacer) OpenOrders(context.Context) ([]domain.Snapshot, error) { return nil, nil }
func (f *fakePlacer) GetOrder(context.Context, domain.Ref) (domain.Snapshot, error) {
	return domain.Snapshot{}, nil
}

type appliedEvent struct {
	source domain.Source
	ev     domain.Event
}

type fakeStore struct {
	mu          sync.Mutex
	pending     []domain.Request
	applied     []appliedEvent
	cancelMarks []domain.ClientOrderID
	stored      ports.StoredOrder
	getErr      error
	applyErr    error
	decision    domain.Decision
	appliedCh   chan struct{}
}

func (f *fakeStore) CreatePending(_ context.Context, req domain.Request) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pending = append(f.pending, req)
	return nil
}

func (f *fakeStore) ApplyEvent(_ context.Context, source domain.Source, ev domain.Event) (domain.Decision, error) {
	f.mu.Lock()
	f.applied = append(f.applied, appliedEvent{source: source, ev: ev})
	f.mu.Unlock()
	if f.appliedCh != nil {
		f.appliedCh <- struct{}{}
	}
	return f.decision, f.applyErr
}

func (f *fakeStore) GetOrder(context.Context, domain.ClientOrderID) (ports.StoredOrder, error) {
	return f.stored, f.getErr
}

func (f *fakeStore) ListActiveOrders(context.Context, instrument.VenueID) ([]ports.StoredOrder, error) {
	return nil, nil
}

func (f *fakeStore) MarkCancelRequested(_ context.Context, orderID domain.ClientOrderID, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancelMarks = append(f.cancelMarks, orderID)
	return nil
}

func newService(t *testing.T, placer ports.OrderPlacer, store ports.OrderStore, streamer ports.PrivateStreamer) (*Service, *clocktest.Clock, *Metrics) {
	t.Helper()
	clk := clocktest.New(time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC))
	m, err := NewMetrics(prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	venues := []Venue{{ID: "bybit", Placer: placer, Streamer: streamer}}
	return New(venues, store, clk, testLogger(t), 2*time.Second, m), clk, m
}

func placeRequest() domain.Request {
	return domain.Request{
		Instrument: testInstrument(),
		Side:       domain.Buy,
		Type:       domain.Limit,
		Price:      decimal.RequireFromString("50000"),
		Qty:        decimal.RequireFromString("1"),
	}
}

func TestPlaceHappyPath(t *testing.T) {
	t.Parallel()

	placer := &fakePlacer{}
	store := &fakeStore{decision: domain.Decision{Transition: true, To: domain.StatusOpen}}
	svc, _, _ := newService(t, placer, store, nil)

	orderID, err := svc.Place(context.Background(), placeRequest())
	if err != nil {
		t.Fatalf("Place: %v", err)
	}
	if len(orderID) != 26 {
		t.Fatalf("assigned ClientOrderID = %q, want a ULID", orderID)
	}
	if len(store.pending) != 1 || store.pending[0].ClientOrderID != orderID || store.pending[0].BotID != "manual" {
		t.Fatalf("pending = %+v", store.pending)
	}
	if len(placer.submits) != 1 || placer.submits[0].ClientOrderID != orderID {
		t.Fatalf("submits = %+v", placer.submits)
	}
	if len(store.applied) != 1 || store.applied[0].source != domain.SourceAck ||
		store.applied[0].ev.Status != domain.StatusOpen || store.applied[0].ev.Ref.VenueOrderID != "v-1" {
		t.Fatalf("applied = %+v", store.applied)
	}
}

func TestPlaceRetriesWithSameID(t *testing.T) {
	t.Parallel()

	placer := &fakePlacer{failures: 2, err: errors.New("venue timeout")}
	store := &fakeStore{}
	svc, _, _ := newService(t, placer, store, nil)

	orderID, err := svc.Place(context.Background(), placeRequest())
	if err != nil {
		t.Fatalf("Place: %v", err)
	}
	if len(placer.submits) != 3 {
		t.Fatalf("submits = %d, want 3", len(placer.submits))
	}
	for i, sub := range placer.submits {
		if sub.ClientOrderID != orderID {
			t.Fatalf("submit %d used ID %q, want %q: the ULID must never regenerate", i, sub.ClientOrderID, orderID)
		}
	}
	if len(store.pending) != 1 {
		t.Fatalf("CreatePending calls = %d, want 1", len(store.pending))
	}
}

func TestPlaceUnsettledKeepsPending(t *testing.T) {
	t.Parallel()

	placer := &fakePlacer{failures: 100, err: ports.ErrAuth}
	store := &fakeStore{}
	svc, _, _ := newService(t, placer, store, nil)

	orderID, err := svc.Place(context.Background(), placeRequest())
	if !errors.Is(err, ErrSubmitUnsettled) {
		t.Fatalf("err = %v, want ErrSubmitUnsettled", err)
	}
	if orderID == "" {
		t.Fatal("ID must be returned even when unsettled")
	}
	if len(store.pending) != 1 || len(store.applied) != 0 {
		t.Fatalf("pending=%d applied=%d, want 1 and 0", len(store.pending), len(store.applied))
	}
}

func TestCancel(t *testing.T) {
	t.Parallel()

	placer := &fakePlacer{}
	store := &fakeStore{stored: ports.StoredOrder{
		ClientOrderID: "cid-1", Instrument: testInstrument(),
		Status: domain.StatusOpen, VenueOrderID: "v-1",
	}}
	svc, _, _ := newService(t, placer, store, nil)

	if err := svc.Cancel(context.Background(), "cid-1"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if len(store.cancelMarks) != 1 || store.cancelMarks[0] != "cid-1" {
		t.Fatalf("cancel marks = %v", store.cancelMarks)
	}
	if len(placer.cancels) != 1 || placer.cancels[0].VenueOrderID != "v-1" {
		t.Fatalf("venue cancels = %+v", placer.cancels)
	}

	store.stored.Status = domain.StatusFilled
	if err := svc.Cancel(context.Background(), "cid-1"); err == nil {
		t.Fatal("cancel of terminal order: want error")
	}
}

type fakeStreamer struct {
	mu     sync.Mutex
	errs   int
	events chan domain.Event
}

// StreamOrderEvents honors the port contract the way the real adapter
// does: the returned channel closes when ctx is canceled.
func (f *fakeStreamer) StreamOrderEvents(ctx context.Context) (<-chan domain.Event, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.errs > 0 {
		f.errs--
		return nil, errors.New("socket refused")
	}
	out := make(chan domain.Event)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-f.events:
				if !ok {
					return
				}
				select {
				case out <- ev:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, nil
}

func streamEvent(status domain.Status) domain.Event {
	return domain.Event{
		Ref: domain.Ref{
			Instrument:    testInstrument(),
			ClientOrderID: "cid-1",
			VenueOrderID:  "v-1",
		},
		Status: status,
		At:     time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC),
	}
}

func TestRunAppliesStreamEventsAndRetriesStream(t *testing.T) {
	t.Parallel()

	streamer := &fakeStreamer{errs: 1, events: make(chan domain.Event, 4)}
	store := &fakeStore{appliedCh: make(chan struct{}, 4)}
	svc, clk, _ := newService(t, &fakePlacer{}, store, streamer)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- svc.Run(ctx) }()

	// The first StreamOrderEvents call fails and the retry waits on the
	// fake clock. Advancing in a loop covers the race between this test
	// and the service registering its After waiter.
	streamer.events <- streamEvent(domain.StatusOpen)
	deadline := time.After(5 * time.Second)
	applied := false
	for !applied {
		select {
		case <-store.appliedCh:
			applied = true
		case <-deadline:
			t.Fatal("stream event was not applied")
		default:
			clk.Advance(streamRetryDelay)
			time.Sleep(time.Millisecond)
		}
	}
	store.mu.Lock()
	if store.applied[0].source != domain.SourceStream || store.applied[0].ev.Status != domain.StatusOpen {
		t.Fatalf("applied = %+v", store.applied[0])
	}
	store.mu.Unlock()

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run returned %v, want nil", err)
	}
}

func TestRunSkipsVenuesWithoutStream(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t, &fakePlacer{}, &fakeStore{}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- svc.Run(ctx) }()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return")
	}
}

func TestApplyCountsDrops(t *testing.T) {
	t.Parallel()

	store := &fakeStore{decision: domain.Decision{To: domain.StatusOpen, Drop: domain.DropDuplicate}}
	svc, _, m := newService(t, &fakePlacer{}, store, nil)

	if err := svc.apply(context.Background(), domain.SourceStream, streamEvent(domain.StatusOpen)); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if got := testutil.ToFloat64(m.dropped.WithLabelValues("bybit", "duplicate")); got != 1 {
		t.Fatalf("dropped{duplicate} = %v, want 1", got)
	}

	store.decision = domain.Decision{Transition: true, To: domain.StatusFilled, FillAnomaly: true}
	if err := svc.apply(context.Background(), domain.SourceStream, streamEvent(domain.StatusFilled)); err != nil {
		t.Fatalf("apply fill anomaly: %v", err)
	}
	if got := testutil.ToFloat64(m.dropped.WithLabelValues("bybit", "negative_fill_delta")); got != 1 {
		t.Fatalf("dropped{negative_fill_delta} = %v, want 1", got)
	}

	store.applyErr = ports.ErrNotFound
	if err := svc.apply(context.Background(), domain.SourceStream, streamEvent(domain.StatusOpen)); err != nil {
		t.Fatalf("apply unknown order: %v", err)
	}
	if got := testutil.ToFloat64(m.dropped.WithLabelValues("bybit", "unknown_order")); got != 1 {
		t.Fatalf("dropped{unknown_order} = %v, want 1", got)
	}

	store.applyErr = errors.New("postgres down")
	if err := svc.apply(context.Background(), domain.SourceStream, streamEvent(domain.StatusOpen)); err == nil {
		t.Fatal("store failure must propagate")
	}
}
