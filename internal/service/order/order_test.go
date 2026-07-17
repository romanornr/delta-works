package order

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/shopspring/decimal"

	"github.com/romanornr/delta-works/internal/domain/instrument"
	domain "github.com/romanornr/delta-works/internal/domain/order"
	"github.com/romanornr/delta-works/internal/log"
	"github.com/romanornr/delta-works/internal/ports"
	"github.com/romanornr/delta-works/internal/ports/portstest"
)

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
	orders   map[domain.ClientOrderID]domain.Snapshot
}

func (f *fakePlacer) PlaceOrder(_ context.Context, req domain.Request) (domain.Ack, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.submits = append(f.submits, req)
	if f.failures > 0 {
		f.failures--
		return domain.Ack{}, f.err
	}
	if existing, ok := f.orders[req.ClientOrderID]; ok {
		return domain.Ack{Ref: existing.Ref, Status: existing.Status, AcceptedAt: existing.UpdatedAt}, nil
	}
	if f.orders == nil {
		f.orders = make(map[domain.ClientOrderID]domain.Snapshot)
	}
	ref := domain.Ref{Instrument: req.Instrument, ClientOrderID: req.ClientOrderID, VenueOrderID: "v-1"}
	f.orders[req.ClientOrderID] = domain.Snapshot{Ref: ref, Status: domain.StatusOpen, Price: req.Price, Qty: req.Qty, UpdatedAt: time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)}
	return domain.Ack{
		Ref:        ref,
		Status:     domain.StatusOpen,
		AcceptedAt: time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC),
	}, nil
}

func (f *fakePlacer) CancelOrder(_ context.Context, ref domain.Ref) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancels = append(f.cancels, ref)
	if stored, ok := f.orders[ref.ClientOrderID]; ok {
		stored.Status = domain.StatusCanceled
		f.orders[ref.ClientOrderID] = stored
	}
	return f.err
}

func (f *fakePlacer) OpenOrders(context.Context) ([]domain.Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var open []domain.Snapshot
	for _, snapshot := range f.orders {
		if !snapshot.Status.Terminal() {
			open = append(open, snapshot)
		}
	}
	return open, nil
}

func (f *fakePlacer) GetOrder(_ context.Context, ref domain.Ref) (domain.Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	stored, ok := f.orders[ref.ClientOrderID]
	if !ok {
		return domain.Snapshot{}, ports.ErrNotFound
	}
	return stored, nil
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
	stored      domain.Record
	getErr      error
	applyErr    error
	result      domain.ApplyResult
	appliedCh   chan struct{}
}

func (f *fakeStore) CreatePending(_ context.Context, req domain.Request) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pending = append(f.pending, req)
	if f.stored.ClientOrderID != "" {
		return false, nil
	}
	f.stored = domain.Record{
		ClientOrderID: req.ClientOrderID,
		BotID:         req.BotID,
		Instrument:    req.Instrument,
		Side:          req.Side,
		Type:          req.Type,
		Price:         req.Price,
		Qty:           req.Qty,
		Status:        domain.StatusPending,
	}
	return true, nil
}

func (f *fakeStore) ApplyEvent(_ context.Context, source domain.Source, ev domain.Event) (domain.ApplyResult, error) {
	f.mu.Lock()
	f.applied = append(f.applied, appliedEvent{source: source, ev: ev})
	if f.applyErr == nil {
		f.stored.Status = ev.Status
		f.stored.VenueOrderID = ev.Ref.VenueOrderID
	}
	f.mu.Unlock()
	if f.appliedCh != nil {
		f.appliedCh <- struct{}{}
	}
	return f.result, f.applyErr
}

func (f *fakeStore) GetOrder(context.Context, domain.ClientOrderID) (domain.Record, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stored, f.getErr
}

func (f *fakeStore) MarkCancelRequested(_ context.Context, orderID domain.ClientOrderID, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancelMarks = append(f.cancelMarks, orderID)
	return nil
}

func newService(t *testing.T, placer ports.OrderPlacer, store *fakeStore, streamer ports.PrivateStreamer) (*Service, *clockwork.FakeClock, *Metrics) {
	t.Helper()
	clk := clockwork.NewFakeClockAt(time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC))
	m, err := NewMetrics(prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	venues := []Venue{{ID: "bybit", Placer: placer, Streamer: streamer}}
	return New(venues, store, store, clk, log.Nop(), 2*time.Second, m), clk, m
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
	store := &fakeStore{result: domain.ApplyResult{Decision: domain.Decision{Transition: true, To: domain.StatusOpen}}}
	svc, _, _ := newService(t, placer, store, nil)

	result, err := svc.Place(context.Background(), placeRequest())
	if err != nil {
		t.Fatalf("Place: %v", err)
	}
	orderID := result.ClientOrderID
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

	result, err := svc.Place(context.Background(), placeRequest())
	if err != nil {
		t.Fatalf("Place: %v", err)
	}
	orderID := result.ClientOrderID
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

	placer := &fakePlacer{failures: 100, err: errors.New("ambiguous timeout")}
	store := &fakeStore{}
	svc, _, _ := newService(t, placer, store, nil)
	svc.submitBudget = time.Millisecond

	result, err := svc.Place(context.Background(), placeRequest())
	if !errors.Is(err, ErrSubmitUnsettled) {
		t.Fatalf("err = %v, want ErrSubmitUnsettled", err)
	}
	orderID := result.ClientOrderID
	if orderID == "" {
		t.Fatal("ID must be returned even when unsettled")
	}
	if len(store.pending) != 1 || len(store.applied) != 0 {
		t.Fatalf("pending=%d applied=%d, want 1 and 0", len(store.pending), len(store.applied))
	}
}

func TestPlacePermanentErrorRejectsOrder(t *testing.T) {
	t.Parallel()
	placer := &fakePlacer{failures: 1, err: ports.ErrAuth}
	store := &fakeStore{}
	svc, _, _ := newService(t, placer, store, nil)
	result, err := svc.Place(t.Context(), placeRequest())
	if !errors.Is(err, ports.ErrAuth) {
		t.Fatalf("err = %v, want ErrAuth", err)
	}
	if result.ClientOrderID == "" || result.Status != domain.StatusRejected {
		t.Fatalf("result = %+v, want rejected with ID", result)
	}
	if len(store.applied) != 1 || store.applied[0].source != domain.SourceLocal ||
		store.applied[0].ev.Status != domain.StatusRejected {
		t.Fatalf("applied = %+v, want one local rejected event", store.applied)
	}
}

func TestPlaceAcceptedButApplyFailedReturnsID(t *testing.T) {
	t.Parallel()
	placer := &fakePlacer{}
	store := &fakeStore{applyErr: errors.New("db down")}
	svc, _, _ := newService(t, placer, store, nil)
	result, err := svc.Place(t.Context(), placeRequest())
	if !errors.Is(err, ErrSubmitUnsettled) {
		t.Fatalf("err = %v, want ErrSubmitUnsettled", err)
	}
	if len(result.ClientOrderID) != 26 {
		t.Fatalf("ClientOrderID = %q, want the generated ULID: the order is live at the venue", result.ClientOrderID)
	}
}

func TestPlaceContextDeadlineIsUnsettled(t *testing.T) {
	t.Parallel()
	placer := &fakePlacer{failures: 100, err: context.DeadlineExceeded}
	store := &fakeStore{}
	svc, _, _ := newService(t, placer, store, nil)
	svc.submitBudget = time.Millisecond
	result, err := svc.Place(t.Context(), placeRequest())
	if !errors.Is(err, ErrSubmitUnsettled) {
		t.Fatalf("err = %v, want ErrSubmitUnsettled: a deadline can race an in-flight submit", err)
	}
	if result.ClientOrderID == "" {
		t.Fatal("ID must be returned even when the deadline fired")
	}
}

func TestPlaceSuppliedIDContract(t *testing.T) {
	t.Parallel()
	request := placeRequest()
	request.ClientOrderID = "01J00000000000000000000001"
	request.BotID = "manual"
	stored := domain.Record{
		ClientOrderID: request.ClientOrderID, BotID: request.BotID, Instrument: request.Instrument,
		Side: request.Side, Type: request.Type, Price: request.Price, Qty: request.Qty, Status: domain.StatusOpen, VenueOrderID: "v-1",
	}
	t.Run("matching advanced order is not resubmitted", func(t *testing.T) {
		placer, store := &fakePlacer{}, &fakeStore{stored: stored}
		svc, _, _ := newService(t, placer, store, nil)
		result, err := svc.Place(t.Context(), request)
		if err != nil || result.Status != domain.StatusOpen || len(placer.submits) != 0 {
			t.Fatalf("result=%+v err=%v submits=%d", result, err, len(placer.submits))
		}
	})
	t.Run("mismatch does not submit", func(t *testing.T) {
		placer, store := &fakePlacer{}, &fakeStore{stored: stored}
		store.stored.Qty = decimal.NewFromInt(2)
		svc, _, _ := newService(t, placer, store, nil)
		_, err := svc.Place(t.Context(), request)
		if !errors.Is(err, ErrIdentityMismatch) || len(placer.submits) != 0 {
			t.Fatalf("err=%v submits=%d", err, len(placer.submits))
		}
	})
	t.Run("matching pending order recovers with same ID", func(t *testing.T) {
		placer, store := &fakePlacer{}, &fakeStore{stored: stored}
		store.stored.Status, store.stored.VenueOrderID = domain.StatusPending, ""
		svc, _, _ := newService(t, placer, store, nil)
		result, err := svc.Place(t.Context(), request)
		if err != nil || result.ClientOrderID != request.ClientOrderID || len(placer.submits) != 1 {
			t.Fatalf("result=%+v err=%v submits=%d", result, err, len(placer.submits))
		}
	})
	t.Run("advanced order answers even after the venue is deconfigured", func(t *testing.T) {
		deconfigured := request
		deconfigured.Instrument.Venue = "kraken"
		storedElsewhere := stored
		storedElsewhere.Instrument.Venue = "kraken"
		placer, store := &fakePlacer{}, &fakeStore{stored: storedElsewhere}
		svc, _, _ := newService(t, placer, store, nil)
		result, err := svc.Place(t.Context(), deconfigured)
		if err != nil || result.Status != domain.StatusOpen || len(placer.submits) != 0 {
			t.Fatalf("result=%+v err=%v submits=%d", result, err, len(placer.submits))
		}
	})
	t.Run("new order on an unconfigured venue is refused", func(t *testing.T) {
		unconfigured := placeRequest()
		unconfigured.Instrument.Venue = "kraken"
		svc, _, _ := newService(t, &fakePlacer{}, &fakeStore{getErr: ports.ErrNotFound}, nil)
		if _, err := svc.Place(t.Context(), unconfigured); !errors.Is(err, ErrVenueNotConfigured) {
			t.Fatalf("err = %v, want ErrVenueNotConfigured", err)
		}
	})
}

func TestConcurrentSameIDPlace(t *testing.T) {
	t.Parallel()
	request := placeRequest()
	request.ClientOrderID = "01J00000000000000000000002"
	request.BotID = "manual"
	placer, store := &fakePlacer{}, &fakeStore{}
	svc, _, _ := newService(t, placer, store, nil)
	var wg sync.WaitGroup
	results := make(chan PlaceResult, 2)
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := svc.Place(t.Context(), request)
			results <- result
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("Place: %v", err)
		}
	}
	for result := range results {
		if result.ClientOrderID != request.ClientOrderID {
			t.Fatalf("result ID = %q", result.ClientOrderID)
		}
	}
	open, err := placer.OpenOrders(t.Context())
	if err != nil || len(open) != 1 || open[0].Ref.VenueOrderID != "v-1" {
		t.Fatalf("venue orders=%+v err=%v", open, err)
	}
}

func TestCancel(t *testing.T) {
	t.Parallel()

	placer := &fakePlacer{}
	store := &fakeStore{stored: domain.Record{
		ClientOrderID: "cid-1", Instrument: testInstrument(),
		Status: domain.StatusOpen, VenueOrderID: "v-1",
	}}
	svc, _, _ := newService(t, placer, store, nil)

	if _, err := svc.Cancel(context.Background(), "cid-1"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if len(store.cancelMarks) != 1 || store.cancelMarks[0] != "cid-1" {
		t.Fatalf("cancel marks = %v", store.cancelMarks)
	}
	if len(placer.cancels) != 1 || placer.cancels[0].VenueOrderID != "v-1" {
		t.Fatalf("venue cancels = %+v", placer.cancels)
	}

	store.stored.Status = domain.StatusFilled
	if _, err := svc.Cancel(context.Background(), "cid-1"); err == nil {
		t.Fatal("cancel of terminal order: want error")
	}
}

type fakeStreamer struct {
	mu     sync.Mutex
	errs   int
	events chan domain.Event
	calls  int
}

// StreamOrderEvents honors the port contract the way the real adapter
// does: the returned channel closes when ctx is canceled.
func (f *fakeStreamer) StreamOrderEvents(ctx context.Context) (<-chan domain.Event, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
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

func (f *fakeStreamer) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
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

	waitCtx, waitCancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer waitCancel()
	if err := clk.BlockUntilContext(waitCtx, 1); err != nil {
		t.Fatalf("wait for stream retry: %v", err)
	}

	streamer.events <- streamEvent(domain.StatusOpen)
	clk.Advance(streamRetryDelay)
	select {
	case <-store.appliedCh:
	case <-time.After(5 * time.Second):
		t.Fatal("stream event was not applied")
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

func TestRunStartsOneConsumerPerVenue(t *testing.T) {
	t.Parallel()
	first := &fakeStreamer{events: make(chan domain.Event)}
	second := &fakeStreamer{events: make(chan domain.Event)}
	metrics, err := NewMetrics(prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	clk := clockwork.NewFakeClockAt(time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC))
	store := &fakeStore{}
	svc := New([]Venue{
		{ID: "bybit", Placer: &fakePlacer{}, Streamer: first},
		{ID: "kraken", Placer: &fakePlacer{}, Streamer: second},
	}, store, store, clk, log.Nop(), time.Second, metrics)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- svc.Run(ctx) }()
	deadline := time.After(time.Second)
	for first.callCount() != 1 || second.callCount() != 1 {
		select {
		case <-deadline:
			t.Fatalf("stream calls = %d, %d", first.callCount(), second.callCount())
		default:
			time.Sleep(time.Millisecond)
		}
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestApplyCountsDrops(t *testing.T) {
	t.Parallel()

	store := &fakeStore{result: domain.ApplyResult{Decision: domain.Decision{To: domain.StatusOpen, Drop: domain.DropDuplicate}}}
	svc, _, m := newService(t, &fakePlacer{}, store, nil)

	if err := svc.apply(context.Background(), domain.SourceStream, streamEvent(domain.StatusOpen)); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if got := testutil.ToFloat64(m.dropped.WithLabelValues("bybit", "duplicate")); got != 1 {
		t.Fatalf("dropped{duplicate} = %v, want 1", got)
	}

	store.result.Decision = domain.Decision{Transition: true, To: domain.StatusFilled, FillAnomaly: true}
	if err := svc.apply(context.Background(), domain.SourceStream, streamEvent(domain.StatusFilled)); err != nil {
		t.Fatalf("apply fill anomaly: %v", err)
	}
	if got := testutil.ToFloat64(m.dropped.WithLabelValues("bybit", "negative_fill_delta")); got != 1 {
		t.Fatalf("dropped{negative_fill_delta} = %v, want 1", got)
	}

	store.result = domain.ApplyResult{}
	store.result.UnmatchedQty = decimal.RequireFromString("0.25")
	if err := svc.apply(context.Background(), domain.SourceStream, streamEvent(domain.StatusPartiallyFilled)); err != nil {
		t.Fatalf("apply unmatched sell: %v", err)
	}
	if got := testutil.ToFloat64(m.unmatched.WithLabelValues("bybit")); got != 1 {
		t.Fatalf("ledger_unmatched_sells_total = %v, want 1", got)
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

func TestFillConflictCountedAndLogged(t *testing.T) {
	t.Parallel()

	store := &fakeStore{result: domain.ApplyResult{FillConflict: true}}
	svc, _, m := newService(t, &fakePlacer{}, store, nil)
	store.result.Decision = domain.Decision{Transition: true, To: domain.StatusPartiallyFilled, FillDelta: decimal.RequireFromString("0.1")}
	if err := svc.apply(context.Background(), domain.SourceStream, streamEvent(domain.StatusPartiallyFilled)); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if got := testutil.ToFloat64(m.dropped.WithLabelValues("bybit", "fill_id_conflict")); got != 1 {
		t.Fatalf("dropped{fill_id_conflict} = %v, want 1", got)
	}
}

func TestFakePlacerContract(t *testing.T) {
	t.Parallel()
	portstest.RunOrderPlacerContract(t, &fakePlacer{}, portstest.Fixture{
		Instrument: testInstrument(), MinQty: decimal.NewFromInt(1), MinNotional: decimal.NewFromInt(1),
		NonMarketablePrice:  func(context.Context) (decimal.Decimal, error) { return decimal.NewFromInt(1), nil },
		EchoesClientOrderID: true, Deadline: 5 * time.Second, Cleanup: portstest.CleanupPlacedOrders,
	})
}
