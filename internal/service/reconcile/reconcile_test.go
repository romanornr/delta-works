package reconcile

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/shopspring/decimal"

	"github.com/romanornr/delta-works/internal/bus"
	"github.com/romanornr/delta-works/internal/domain/instrument"
	domain "github.com/romanornr/delta-works/internal/domain/order"
	"github.com/romanornr/delta-works/internal/events"
	"github.com/romanornr/delta-works/internal/log"
	"github.com/romanornr/delta-works/internal/ports"
	orderservice "github.com/romanornr/delta-works/internal/service/order"
)

const testInterval = 30 * time.Second

var testStart = time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)

type fakePlacer struct {
	mu         sync.Mutex
	openOrders []domain.Snapshot
	openErr    error
	getOrders  map[string]fakeOrderResult
	openCalls  int
	getCalls   int
}

type fakeOrderResult struct {
	snapshot domain.Snapshot
	err      error
}

func (*fakePlacer) PlaceOrder(context.Context, domain.Request) (domain.Ack, error) {
	return domain.Ack{}, nil
}

func (*fakePlacer) CancelOrder(context.Context, domain.Ref) error { return nil }

func (f *fakePlacer) OpenOrders(context.Context) ([]domain.Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.openCalls++
	return append([]domain.Snapshot(nil), f.openOrders...), f.openErr
}

func (f *fakePlacer) GetOrder(_ context.Context, ref domain.Ref) (domain.Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getCalls++
	result, ok := f.getOrders[orderRefKey(ref)]
	if !ok {
		return domain.Snapshot{}, errors.New("unexpected point lookup")
	}
	return result.snapshot, result.err
}

func orderRefKey(ref domain.Ref) string {
	return ref.Instrument.Key() + "\x00" + string(ref.ClientOrderID) + "\x00" + ref.VenueOrderID
}

func fakeOrderLookup(ref domain.Ref, snapshot domain.Snapshot, err error) map[string]fakeOrderResult {
	return map[string]fakeOrderResult{orderRefKey(ref): {snapshot: snapshot, err: err}}
}

func (f *fakePlacer) setOpenError(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.openErr = err
}

func (f *fakePlacer) calls() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.openCalls, f.getCalls
}

type appliedEvent struct {
	source domain.Source
	event  domain.Event
}

type fakeStore struct {
	mu       sync.Mutex
	active   []domain.Record
	listErr  error
	stored   domain.Record
	getErr   error
	applyErr error
	result   domain.ApplyResult
	applied  []appliedEvent
}

func (f *fakeStore) ApplyEvent(_ context.Context, source domain.Source, event domain.Event) (domain.ApplyResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.applied = append(f.applied, appliedEvent{source: source, event: event})
	return f.result, f.applyErr
}

func (f *fakeStore) GetOrder(context.Context, domain.ClientOrderID) (domain.Record, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stored, f.getErr
}

func (f *fakeStore) ListActiveOrders(context.Context, instrument.VenueID) ([]domain.Record, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]domain.Record(nil), f.active...), f.listErr
}

func (f *fakeStore) appliedEvents() []appliedEvent {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]appliedEvent(nil), f.applied...)
}

type recordingBus struct {
	mu        sync.Mutex
	prefix    string
	handler   bus.Handler
	published []bus.Event
}

func (b *recordingBus) Publish(ctx context.Context, event bus.Event) error {
	b.mu.Lock()
	b.published = append(b.published, event)
	prefix, handler := b.prefix, b.handler
	b.mu.Unlock()
	if handler != nil && strings.HasPrefix(event.Subject, prefix) {
		handler(ctx, event)
	}
	return nil
}

func (b *recordingBus) Subscribe(prefix string, handler bus.Handler) (func(), error) {
	b.mu.Lock()
	b.prefix, b.handler = prefix, handler
	b.mu.Unlock()
	return func() {
		b.mu.Lock()
		b.handler = nil
		b.mu.Unlock()
	}, nil
}

func (b *recordingBus) events(subject string) []bus.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	var events []bus.Event
	for _, event := range b.published {
		if event.Subject == subject {
			events = append(events, event)
		}
	}
	return events
}

func testInstrument() instrument.Instrument {
	return instrument.Instrument{
		Venue: "bybit", Type: instrument.TypeSpot,
		Base: "BTC", Quote: "USDT", VenueSymbol: "BTCUSDT",
	}
}

func testStored(status domain.Status, age time.Duration) domain.Record {
	return domain.Record{
		ClientOrderID: "cid-1", Instrument: testInstrument(), Status: status,
		FilledQty: decimal.RequireFromString("0.4"), VenueOrderID: "v-1",
		CreatedAt: testStart.Add(-age),
	}
}

func pendingStored(venueOrderID string) domain.Record {
	stored := testStored(domain.StatusPending, time.Minute)
	stored.FilledQty = decimal.Zero
	stored.VenueOrderID = venueOrderID
	return stored
}

func testSnapshot(status domain.Status, filled string) domain.Snapshot {
	return domain.Snapshot{
		Ref: domain.Ref{
			Instrument: testInstrument(), ClientOrderID: "cid-1", VenueOrderID: "v-1",
		},
		Status: status, FilledQty: decimal.RequireFromString(filled),
		AvgFillPrice: decimal.RequireFromString("50100"), UpdatedAt: testStart,
	}
}

func newTestService(t *testing.T, placer *fakePlacer, store *fakeStore) (*Service, *clockwork.FakeClock, *Metrics, *recordingBus) {
	t.Helper()
	metrics, err := NewMetrics(prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	clk := clockwork.NewFakeClockAt(testStart)
	eventBus := &recordingBus{}
	service := New([]Venue{{ID: "bybit", Placer: placer}}, store, store, eventBus, clk, log.Nop(), testInterval, metrics)
	return service, clk, metrics, eventBus
}

func TestPassReconcilesPresentOrders(t *testing.T) {
	tests := []struct {
		name        string
		local       domain.Record
		snapshot    domain.Snapshot
		wantApplied int
		wantKind    string
	}{
		{
			name: "drift", local: testStored(domain.StatusPartiallyFilled, time.Minute),
			snapshot: testSnapshot(domain.StatusPartiallyFilled, "0.6"), wantApplied: 1, wantKind: "drift",
		},
		{
			name: "equal", local: testStored(domain.StatusPartiallyFilled, time.Minute),
			snapshot: testSnapshot(domain.StatusPartiallyFilled, "0.4"), wantApplied: 0,
		},
		{
			name: "equal pending adopts venue order ID", local: pendingStored(""),
			snapshot: testSnapshot(domain.StatusPending, "0"), wantApplied: 1, wantKind: "adopted",
		},
		{
			name: "equal pending with matching venue order ID", local: pendingStored("v-1"),
			snapshot: testSnapshot(domain.StatusPending, "0"), wantApplied: 0,
		},
		{
			name: "pending adopted", local: testStored(domain.StatusPending, time.Minute),
			snapshot: testSnapshot(domain.StatusOpen, "0"), wantApplied: 1, wantKind: "adopted",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			placer := &fakePlacer{openOrders: []domain.Snapshot{tt.snapshot}}
			store := &fakeStore{active: []domain.Record{tt.local}}
			service, _, metrics, _ := newTestService(t, placer, store)
			if err := service.pass(context.Background(), service.venues[0]); err != nil {
				t.Fatalf("pass: %v", err)
			}
			applied := store.appliedEvents()
			if len(applied) != tt.wantApplied {
				t.Fatalf("applied events = %d, want %d", len(applied), tt.wantApplied)
			}
			if tt.wantApplied == 0 {
				return
			}
			if applied[0].source != domain.SourceReconcile ||
				!applied[0].event.FillPrice.Equal(tt.snapshot.AvgFillPrice) {
				t.Fatalf("applied event = %+v", applied[0])
			}
			if got := testutil.ToFloat64(metrics.diffs.WithLabelValues("bybit", tt.wantKind)); got != 1 {
				t.Fatalf("diffs{%s} = %v, want 1", tt.wantKind, got)
			}
		})
	}
}

func TestPendingAbsentRequiresAuthoritativeProof(t *testing.T) {
	tests := []struct {
		name         string
		age          time.Duration
		venueOrderID string
		lookup       *fakeOrderResult
		wantGet      int
		wantApplied  int
		wantKind     string
	}{
		{name: "inside grace", age: testInterval, venueOrderID: "v-1"},
		{
			name: "missing venue order ID remains unresolved", age: 2 * testInterval,
			wantKind: "unresolved_submit",
		},
		{
			name: "venue ID not found remains unresolved", age: 2 * testInterval,
			venueOrderID: "v-1", lookup: &fakeOrderResult{err: ports.ErrNotFound},
			wantGet: 1, wantKind: "unresolved_submit",
		},
		{
			name: "generic error leaves pending", age: 2 * testInterval,
			venueOrderID: "v-1", lookup: &fakeOrderResult{err: errors.New("venue down")},
			wantGet: 1,
		},
		{
			name: "successful venue ID lookup adopts", age: 2 * testInterval,
			venueOrderID: "v-1", lookup: &fakeOrderResult{snapshot: testSnapshot(domain.StatusOpen, "0")},
			wantGet: 1, wantApplied: 1, wantKind: "adopted",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stored := pendingStored(tt.venueOrderID)
			stored.CreatedAt = testStart.Add(-tt.age)
			placer := &fakePlacer{}
			if tt.lookup != nil {
				placer.getOrders = fakeOrderLookup(storedRef(stored), tt.lookup.snapshot, tt.lookup.err)
			}
			store := &fakeStore{active: []domain.Record{stored}}
			service, _, metrics, _ := newTestService(t, placer, store)
			if err := service.pass(context.Background(), service.venues[0]); err != nil {
				t.Fatalf("pass: %v", err)
			}
			_, getCalls := placer.calls()
			if getCalls != tt.wantGet || len(store.appliedEvents()) != tt.wantApplied {
				t.Fatalf("GetOrder calls = %d, applied = %d; want %d, %d", getCalls, len(store.appliedEvents()), tt.wantGet, tt.wantApplied)
			}
			if tt.wantApplied > 0 && store.appliedEvents()[0].event.Status != domain.StatusOpen {
				t.Fatalf("applied event = %+v, want open adoption", store.appliedEvents()[0].event)
			}
			if tt.wantKind != "" && testutil.ToFloat64(metrics.diffs.WithLabelValues("bybit", tt.wantKind)) != 1 {
				got := testutil.ToFloat64(metrics.diffs.WithLabelValues("bybit", tt.wantKind))
				t.Fatalf("diffs{%s} = %v, want 1", tt.wantKind, got)
			}
		})
	}
}

func TestActiveAbsentAppliesTerminalSnapshot(t *testing.T) {
	// The point-lookup snapshot omits our identity keys, as some venues do
	// in single-order responses; the local row must supply them.
	lookup := testSnapshot(domain.StatusFilled, "1")
	lookup.Ref.ClientOrderID = ""
	lookup.Ref.VenueOrderID = ""
	stored := testStored(domain.StatusOpen, time.Minute)
	placer := &fakePlacer{getOrders: fakeOrderLookup(storedRef(stored), lookup, nil)}
	store := &fakeStore{active: []domain.Record{stored}}
	service, _, metrics, _ := newTestService(t, placer, store)
	if err := service.pass(context.Background(), service.venues[0]); err != nil {
		t.Fatalf("pass: %v", err)
	}
	applied := store.appliedEvents()
	if len(applied) != 1 || applied[0].event.Status != domain.StatusFilled {
		t.Fatalf("applied = %+v", applied)
	}
	if ref := applied[0].event.Ref; ref.ClientOrderID != "cid-1" || ref.VenueOrderID != "v-1" {
		t.Fatalf("applied ref = %+v, want local identity keys restored", ref)
	}
	if got := testutil.ToFloat64(metrics.diffs.WithLabelValues("bybit", "closed")); got != 1 {
		t.Fatalf("diffs{closed} = %v, want 1", got)
	}
}

func TestOrphanPublishesOnceAcrossPasses(t *testing.T) {
	orphan := testSnapshot(domain.StatusOpen, "0")
	orphan.Ref.ClientOrderID = "foreign-1"
	placer := &fakePlacer{openOrders: []domain.Snapshot{orphan}}
	store := &fakeStore{getErr: ports.ErrNotFound}
	service, _, metrics, eventBus := newTestService(t, placer, store)
	for range 2 {
		if err := service.pass(context.Background(), service.venues[0]); err != nil {
			t.Fatalf("pass: %v", err)
		}
	}
	published := eventBus.events(events.SubjectReconcileOrphan)
	if len(published) != 1 {
		t.Fatalf("orphan events = %d, want 1", len(published))
	}
	payload, ok := published[0].Payload.(events.ReconcileOrphanPayload)
	if !ok || payload.VenueOrderID != "v-1" || payload.ClientOrderID != "foreign-1" {
		t.Fatalf("orphan payload = %#v", published[0].Payload)
	}
	if got := testutil.ToFloat64(metrics.orphans.WithLabelValues("bybit")); got != 1 {
		t.Fatalf("orphans gauge = %v, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.diffs.WithLabelValues("bybit", "orphan")); got != 2 {
		t.Fatalf("diffs{orphan} = %v, want 2", got)
	}
}

func TestOpenOrdersErrorDoesNotStopRunOrAdvanceSuccess(t *testing.T) {
	placer := &fakePlacer{openErr: errors.New("venue down")}
	store := &fakeStore{}
	service, _, metrics, eventBus := newTestService(t, placer, store)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- service.Run(ctx) }()
	waitForOpenCalls(t, placer, 1)
	if got := testutil.ToFloat64(metrics.lastSuccess.WithLabelValues("bybit")); got != 0 {
		t.Fatalf("last success = %v, want 0", got)
	}
	placer.setOpenError(nil)
	if err := eventBus.Publish(context.Background(), bus.Event{
		Subject: orderservice.SubjectStreamReconnected, Payload: instrument.VenueID("bybit"),
	}); err != nil {
		t.Fatal(err)
	}
	waitForOpenCalls(t, placer, 2)
	waitForGauge(t, func() float64 {
		return testutil.ToFloat64(metrics.lastSuccess.WithLabelValues("bybit"))
	}, float64(testStart.Unix()))
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestStoreListErrorStopsRun(t *testing.T) {
	want := errors.New("postgres down")
	service, _, _, _ := newTestService(t, &fakePlacer{}, &fakeStore{listErr: want})
	if err := service.Run(context.Background()); !errors.Is(err, want) {
		t.Fatalf("Run error = %v, want %v", err, want)
	}
}

func TestReconnectKickTriggersPass(t *testing.T) {
	placer := &fakePlacer{}
	service, _, _, eventBus := newTestService(t, placer, &fakeStore{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- service.Run(ctx) }()
	waitForOpenCalls(t, placer, 1)
	if err := eventBus.Publish(context.Background(), bus.Event{
		Subject: orderservice.SubjectStreamReconnected, Payload: instrument.VenueID("bybit"),
	}); err != nil {
		t.Fatal(err)
	}
	waitForOpenCalls(t, placer, 2)
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestIntervalTriggersPass(t *testing.T) {
	placer := &fakePlacer{}
	service, clk, _, _ := newTestService(t, placer, &fakeStore{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- service.Run(ctx) }()
	waitCtx, waitCancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer waitCancel()
	if err := clk.BlockUntilContext(waitCtx, 1); err != nil {
		t.Fatalf("wait for reconcile interval: %v", err)
	}
	clk.Advance(testInterval)
	if err := clk.BlockUntilContext(waitCtx, 1); err != nil {
		t.Fatalf("wait for next reconcile interval: %v", err)
	}
	openCalls, _ := placer.calls()
	if openCalls != 2 {
		t.Fatalf("OpenOrders calls = %d, want 2", openCalls)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}
}

func TestRunReturnsPromptlyOnCancel(t *testing.T) {
	placer := &fakePlacer{}
	service, _, _, _ := newTestService(t, placer, &fakeStore{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- service.Run(ctx) }()
	waitForOpenCalls(t, placer, 1)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}
}

func waitForOpenCalls(t *testing.T, placer *fakePlacer, want int) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		openCalls, _ := placer.calls()
		if openCalls >= want {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("OpenOrders calls = %d, want at least %d", openCalls, want)
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

func waitForGauge(t *testing.T, value func() float64, want float64) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		got := value()
		if got == want {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("gauge = %v, want %v", got, want)
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

func TestApplyOutcomeCountsAsDiff(t *testing.T) {
	unmatched := domain.ApplyResult{}
	unmatched.UnmatchedQty = decimal.RequireFromString("0.1")
	tests := []struct {
		name, kind string
		snapshot   domain.Snapshot
		result     domain.ApplyResult
	}{
		{
			name: "fill anomaly", snapshot: testSnapshot(domain.StatusCanceled, "0"), kind: "fill_anomaly",
			result: domain.ApplyResult{Decision: domain.Decision{Transition: true, To: domain.StatusCanceled, FillAnomaly: true}},
		},
		{name: "unmatched sell", snapshot: testSnapshot(domain.StatusPartiallyFilled, "0.9"), result: unmatched, kind: "unmatched_sell"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			placer := &fakePlacer{openOrders: []domain.Snapshot{tt.snapshot}}
			store := &fakeStore{active: []domain.Record{testStored(domain.StatusPartiallyFilled, time.Minute)}, result: tt.result}
			service, _, metrics, _ := newTestService(t, placer, store)
			if err := service.pass(context.Background(), service.venues[0]); err != nil {
				t.Fatalf("pass: %v", err)
			}
			if got := testutil.ToFloat64(metrics.diffs.WithLabelValues("bybit", tt.kind)); got != 1 {
				t.Fatalf("diffs{%s} = %v, want 1", tt.kind, got)
			}
		})
	}
}
