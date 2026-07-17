package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/romanornr/delta-works/internal/bus"
	"github.com/romanornr/delta-works/internal/log"
	"github.com/romanornr/delta-works/internal/ports"
)

const testInterval = 500 * time.Millisecond

// fakeStore serves rows from a slice and records every call on a channel
// so tests can synchronize with the relay goroutine.
type fakeStore struct {
	mu            sync.Mutex
	pending       []ports.OutboxMessage
	deletedBefore time.Time
	err           error
	calls         chan string
}

func newFakeStore() *fakeStore {
	return &fakeStore{calls: make(chan string, 100)}
}

func (f *fakeStore) add(msgs ...ports.OutboxMessage) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pending = append(f.pending, msgs...)
}

func (f *fakeStore) PublishPending(_ context.Context, limit int, publish func(ports.OutboxMessage) error) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	defer func() { f.calls <- "publish" }()
	if f.err != nil {
		return 0, f.err
	}
	n := min(limit, len(f.pending))
	for _, m := range f.pending[:n] {
		if err := publish(m); err != nil {
			return 0, err
		}
	}
	f.pending = f.pending[n:]
	return n, nil
}

func (f *fakeStore) UnpublishedStats(context.Context) (int64, time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	defer func() { f.calls <- "stats" }()
	if f.err != nil {
		return 0, time.Time{}, f.err
	}
	var oldest time.Time
	if len(f.pending) > 0 {
		oldest = f.pending[0].CreatedAt
	}
	return int64(len(f.pending)), oldest, nil
}

func (f *fakeStore) DeletePublishedBefore(_ context.Context, cutoff time.Time) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	defer func() { f.calls <- "delete" }()
	if f.err != nil {
		return 0, f.err
	}
	f.deletedBefore = cutoff
	return 1, nil
}

func waitCall(t *testing.T, ch <-chan string, want string) {
	t.Helper()
	for {
		select {
		case got := <-ch:
			if got == want {
				return
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for %q call", want)
		}
	}
}

func msg(id int64) ports.OutboxMessage {
	return ports.OutboxMessage{
		ID:      id,
		Subject: "order.updated",
		Payload: []byte(fmt.Sprintf(`{"id":%d}`, id)),
		// An hour before the fake clock's start so backlog age is nonzero.
		CreatedAt: time.Date(2026, 7, 5, 11, 0, 0, 0, time.UTC),
	}
}

type fixture struct {
	clk    *clockwork.FakeClock
	m      *Metrics
	events chan bus.Event
	done   chan error
	cancel context.CancelFunc
}

func startService(t *testing.T, store ports.OutboxStore, batch int) *fixture {
	t.Helper()
	b := bus.NewInProc()
	t.Cleanup(b.Close)
	events := make(chan bus.Event, 100)
	if _, err := b.Subscribe("order.", func(_ context.Context, e bus.Event) {
		events <- e
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	clk := clockwork.NewFakeClockAt(time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	m, err := NewMetrics(prometheus.NewRegistry())
	if err != nil {
		t.Fatalf("NewMetrics: %v", err)
	}
	svc := New(store, b, clk, log.Nop(), testInterval, batch, m)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := make(chan error, 1)
	go func() { done <- svc.Run(ctx) }()
	return &fixture{clk: clk, m: m, events: events, done: done, cancel: cancel}
}

func (f *fixture) waitEvents(t *testing.T, n int) []bus.Event {
	t.Helper()
	got := make([]bus.Event, 0, n)
	for len(got) < n {
		select {
		case e := <-f.events:
			got = append(got, e)
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out after %d of %d events", len(got), n)
		}
	}
	return got
}

func (f *fixture) stop(t *testing.T) {
	t.Helper()
	f.cancel()
	if err := <-f.done; err != nil {
		t.Fatalf("Run returned %v, want nil", err)
	}
}

func TestDrainsBurstAcrossBatches(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	for i := range 5 {
		store.add(msg(int64(i + 1)))
	}
	f := startService(t, store, 2)

	// One drain clears the whole burst: batches of 2, 2, 1 without
	// waiting a poll interval between them.
	got := f.waitEvents(t, 5)
	for i, e := range got {
		if e.Subject != "order.updated" {
			t.Fatalf("event %d subject = %q", i, e.Subject)
		}
		raw, ok := e.Payload.(json.RawMessage)
		if want := fmt.Sprintf(`{"id":%d}`, i+1); !ok || string(raw) != want {
			t.Fatalf("event %d payload = %s, want %s", i, raw, want)
		}
	}
	f.stop(t)
}

func TestPublishesOnTick(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.add(msg(1))
	f := startService(t, store, 100)

	f.waitEvents(t, 1)
	waitCall(t, store.calls, "stats")

	store.add(msg(2))
	f.clk.Advance(testInterval)
	got := f.waitEvents(t, 1)
	if raw, ok := got[0].Payload.(json.RawMessage); !ok || string(raw) != `{"id":2}` {
		t.Fatalf("payload = %s, want {\"id\":2}", raw)
	}
	f.stop(t)
}

func TestCleanupUsesRetentionCutoff(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	f := startService(t, store, 100)
	waitCall(t, store.calls, "stats")

	f.clk.Advance(cleanupInterval)
	waitCall(t, store.calls, "delete")

	store.mu.Lock()
	cutoff := store.deletedBefore
	store.mu.Unlock()
	want := f.clk.Now().Add(-retention)
	if !cutoff.Equal(want) {
		t.Fatalf("cutoff = %v, want %v", cutoff, want)
	}
	f.stop(t)
}

func TestBacklogMetrics(t *testing.T) {
	t.Parallel()

	// A store whose rows never drain: PublishPending reports zero so the
	// backlog gauges must reflect the stuck rows.
	store := newFakeStore()
	stuck := &stuckStore{fakeStore: store}
	stuck.add(msg(1))
	f := startService(t, stuck, 100)

	// The stats call returns before the service goroutine sets the
	// gauges, so poll instead of synchronizing on the store call.
	deadline := time.Now().Add(5 * time.Second)
	for {
		rows := testutil.ToFloat64(f.m.unpublished)
		age := testutil.ToFloat64(f.m.oldestAge)
		if rows == 1 && age > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("gauges = %v rows, %vs age; want 1 row and positive age", rows, age)
		}
		time.Sleep(10 * time.Millisecond)
	}
	f.stop(t)
}

// stuckStore keeps rows pending forever, as if every claim found the rows
// locked by another transaction.
type stuckStore struct {
	*fakeStore
}

func (s *stuckStore) PublishPending(context.Context, int, func(ports.OutboxMessage) error) (int, error) {
	s.calls <- "publish"
	return 0, nil
}

func TestStoreFailureStopsService(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.err = errors.New("postgres down")
	f := startService(t, store, 100)

	select {
	case err := <-f.done:
		if err == nil || !errors.Is(err, store.err) {
			t.Fatalf("Run returned %v, want wrapped store error", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop on store failure")
	}
}
