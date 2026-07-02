package snapshot

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/shopspring/decimal"

	"github.com/romanornr/delta-works/internal/bus"
	"github.com/romanornr/delta-works/internal/clock/clocktest"
	"github.com/romanornr/delta-works/internal/config"
	"github.com/romanornr/delta-works/internal/domain/account"
	"github.com/romanornr/delta-works/internal/domain/instrument"
	"github.com/romanornr/delta-works/internal/domain/marketdata"
	"github.com/romanornr/delta-works/internal/exchange"
	"github.com/romanornr/delta-works/internal/log"
	"github.com/romanornr/delta-works/internal/ports"
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
	c    ports.SnapshotCheckpoint
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

func (f *fakeStores) WriteTicker(context.Context, marketdata.Ticker) error { return nil }

func (f *fakeStores) Flush(context.Context) error {
	f.add(call{kind: "flush"})
	return nil
}

func (f *fakeStores) RecordSnapshot(_ context.Context, c ports.SnapshotCheckpoint) error {
	f.add(call{kind: "checkpoint", c: c})
	return nil
}

func (f *fakeStores) LastSnapshot(context.Context, account.Ref) (ports.SnapshotCheckpoint, error) {
	return ports.SnapshotCheckpoint{}, ports.ErrNotFound
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

func testLogger(t *testing.T) log.Logger {
	t.Helper()
	logger, err := log.New(config.Log{Level: "error", Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	return logger
}

func TestSnapshotOrderingAndTick(t *testing.T) {
	start := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	clk := clocktest.New(start)
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

	svc := New(reg, stores, stores, b, clk, testLogger(t), time.Minute,
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
	if cp.c.Status != ports.CheckpointOK || cp.c.BalanceCount != 1 {
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
	clk := clocktest.New(time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC))
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

	svc := New(reg, stores, stores, b, clk, testLogger(t), time.Minute,
		[]Target{{Venue: "bybit", Account: account.TypeSpot}}, m)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- svc.Run(ctx) }()

	cp := waitCall(t, stores.ch, "checkpoint")
	if cp.c.Status != ports.CheckpointFailed || cp.c.Error == "" {
		t.Errorf("expected failed checkpoint with error, got %+v", cp.c)
	}

	cancel()
	if err := <-done; err != nil {
		t.Errorf("Run returned error: %v", err)
	}
}

func TestCheckpointStoreFailureStopsService(t *testing.T) {
	clk := clocktest.New(time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC))
	stores := &failingCheckpoints{fakeStores: newFakeStores()}
	reg := exchange.NewRegistry([]ports.Exchange{&fakeExchange{}})
	b := bus.NewInProc()
	defer b.Close()
	m, err := NewMetrics(prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}

	svc := New(reg, stores, stores, b, clk, testLogger(t), time.Minute,
		[]Target{{Venue: "bybit", Account: account.TypeSpot}}, m)

	if err := svc.Run(context.Background()); err == nil {
		t.Error("expected Run to fail when the checkpoint store is down")
	}
}

type failingCheckpoints struct {
	*fakeStores
}

func (f *failingCheckpoints) RecordSnapshot(context.Context, ports.SnapshotCheckpoint) error {
	return errors.New("postgres down")
}
