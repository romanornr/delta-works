package snapshot

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

	"github.com/romanornr/delta-works/internal/bus"
	"github.com/romanornr/delta-works/internal/domain/account"
	"github.com/romanornr/delta-works/internal/domain/instrument"
	"github.com/romanornr/delta-works/internal/domain/marketdata"
	"github.com/romanornr/delta-works/internal/exchange"
	"github.com/romanornr/delta-works/internal/log"
	"github.com/romanornr/delta-works/internal/ports"
	snapshotmodel "github.com/romanornr/delta-works/internal/snapshot"
)

type fakeExchange struct {
	mu  sync.Mutex
	err error
}

func (f *fakeExchange) ID() instrument.VenueID { return "bybit" }

func (f *fakeExchange) Ticker(context.Context, instrument.Instrument) (marketdata.Ticker, error) {
	return marketdata.Ticker{}, nil
}

func (f *fakeExchange) Instruments(context.Context, instrument.Type) ([]instrument.Instrument, error) {
	return nil, nil
}

func (f *fakeExchange) Balances(context.Context, account.Type) ([]account.Balance, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	return []account.Balance{{Currency: "BTC", Total: decimal.NewFromInt(1), Free: decimal.NewFromInt(1)}}, nil
}

type call struct {
	kind string // "write", "flush", "checkpoint"
	c    snapshotmodel.Checkpoint
}

type fakeStores struct {
	mu    sync.Mutex
	calls []call
	ch    chan call
}

func newFakeStores() *fakeStores { return &fakeStores{ch: make(chan call, 16)} }

func (f *fakeStores) add(c call) {
	f.mu.Lock()
	f.calls = append(f.calls, c)
	f.mu.Unlock()
	f.ch <- c
}

func (f *fakeStores) WriteBalanceSnapshot(_ context.Context, _ account.Snapshot) error {
	f.add(call{kind: "write"})
	return nil
}

func (f *fakeStores) Flush(context.Context) error {
	f.add(call{kind: "flush"})
	return nil
}

func (f *fakeStores) RecordSnapshot(_ context.Context, c snapshotmodel.Checkpoint) error {
	f.add(call{kind: "checkpoint", c: c})
	return nil
}

func waitCall(t *testing.T, ch chan call, kind string) call {
	t.Helper()
	for {
		select {
		case c := <-ch:
			if c.kind == kind {
				return c
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for %s", kind)
		}
	}
}

func TestSnapshotOrderingAndTick(t *testing.T) {
	start := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	clk := clockwork.NewFakeClockAt(start)
	stores := newFakeStores()
	fake := &fakeExchange{}
	reg := exchange.NewRegistry([]ports.Exchange{fake})
	b := bus.NewInProc()
	defer b.Close()
	m, err := NewMetrics(prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}

	events := make(chan bus.Event, 4)
	if _, err := b.Subscribe(SubjectTaken, func(_ context.Context, e bus.Event) { events <- e }); err != nil {
		t.Fatal(err)
	}

	svc := New(reg, stores, stores, b, clk, log.Nop(), time.Minute,
		[]Target{{Venue: "bybit", Account: account.TypeSpot}}, m)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- svc.Run(ctx) }()

	// Startup snapshot: write, then flush, then checkpoint.
	if c := waitCall(t, stores.ch, "write"); c.kind != "write" {
		t.Fatal("expected write first")
	}
	waitCall(t, stores.ch, "flush")
	cp := waitCall(t, stores.ch, "checkpoint")
	if cp.c.Status != snapshotmodel.StatusOK || cp.c.BalanceCount != 1 {
		t.Errorf("checkpoint: got %+v", cp.c)
	}
	if cp.c.TakenAt != start {
		t.Errorf("TakenAt: got %s, want fake clock time %s", cp.c.TakenAt, start)
	}

	select {
	case e := <-events:
		if e.Subject != SubjectTaken {
			t.Errorf("event subject: %s", e.Subject)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no snapshot.taken event")
	}

	// Advancing the fake clock by one interval triggers the next snapshot.
	clk.Advance(time.Minute)
	waitCall(t, stores.ch, "checkpoint")

	cancel()
	if err := <-done; err != nil {
		t.Errorf("Run returned error: %v", err)
	}

	stores.mu.Lock()
	defer stores.mu.Unlock()
	order := []string{}
	for _, c := range stores.calls[:3] {
		order = append(order, c.kind)
	}
	if order[0] != "write" || order[1] != "flush" || order[2] != "checkpoint" {
		t.Errorf("persist order wrong: %v", order)
	}
}

func TestFailedFetchRecordsFailedCheckpoint(t *testing.T) {
	clk := clockwork.NewFakeClockAt(time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC))
	stores := newFakeStores()
	// ErrAuth is permanent: no retry loop, immediate failed checkpoint.
	fake := &fakeExchange{err: ports.ErrAuth}
	reg := exchange.NewRegistry([]ports.Exchange{fake})
	b := bus.NewInProc()
	defer b.Close()
	m, err := NewMetrics(prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}

	svc := New(reg, stores, stores, b, clk, log.Nop(), time.Minute,
		[]Target{{Venue: "bybit", Account: account.TypeSpot}}, m)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- svc.Run(ctx) }()

	cp := waitCall(t, stores.ch, "checkpoint")
	if cp.c.Status != snapshotmodel.StatusFailed || cp.c.Error == "" {
		t.Errorf("expected failed checkpoint with error, got %+v", cp.c)
	}

	cancel()
	if err := <-done; err != nil {
		t.Errorf("Run returned error: %v", err)
	}
}

func TestCheckpointStoreFailureStopsService(t *testing.T) {
	clk := clockwork.NewFakeClockAt(time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC))
	stores := &failingCheckpoints{fakeStores: newFakeStores()}
	reg := exchange.NewRegistry([]ports.Exchange{&fakeExchange{}})
	b := bus.NewInProc()
	defer b.Close()
	m, err := NewMetrics(prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}

	svc := New(reg, stores, stores, b, clk, log.Nop(), time.Minute,
		[]Target{{Venue: "bybit", Account: account.TypeSpot}}, m)

	if err := svc.Run(context.Background()); err == nil {
		t.Error("expected Run to fail when the checkpoint store is down")
	}
}

type failingCheckpoints struct {
	*fakeStores
}

func (f *failingCheckpoints) RecordSnapshot(context.Context, snapshotmodel.Checkpoint) error {
	return errors.New("postgres down")
}

type deadlineCheckpoints struct{}

func (*deadlineCheckpoints) RecordSnapshot(ctx context.Context, _ snapshotmodel.Checkpoint) error {
	<-ctx.Done()
	return ctx.Err()
}

type recordingBus struct {
	mu     sync.Mutex
	events []bus.Event
}

func (b *recordingBus) Publish(_ context.Context, event bus.Event) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, event)
	return nil
}

func (*recordingBus) Subscribe(string, bus.Handler) (func(), error) { return func() {}, nil }

func (b *recordingBus) count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.events)
}

func TestCheckpointDeadlineDoesNotReportSuccess(t *testing.T) {
	clk := clockwork.NewFakeClockAt(time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC))
	series := newFakeStores()
	eventBus := &recordingBus{}
	metrics, err := NewMetrics(prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	target := Target{Venue: "bybit", Account: account.TypeSpot}
	svc := New(
		exchange.NewRegistry([]ports.Exchange{&fakeExchange{}}),
		series,
		&deadlineCheckpoints{},
		eventBus,
		clk,
		log.Nop(),
		time.Minute,
		[]Target{target},
		metrics,
	)

	err = svc.snapshot(context.Background(), target)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("snapshot error = %v, want DeadlineExceeded", err)
	}
	if got := testutil.CollectAndCount(metrics.duration); got != 0 {
		t.Fatalf("snapshot duration observations = %d, want 0", got)
	}
	if got := testutil.CollectAndCount(metrics.lastSuccess); got != 0 {
		t.Fatalf("snapshot last-success observations = %d, want 0", got)
	}
	if got := eventBus.count(); got != 0 {
		t.Fatalf("published events = %d, want 0", got)
	}
}
