package gct

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	gctstream "github.com/thrasher-corp/gocryptotrader/exchange/stream"

	"github.com/romanornr/delta-works/internal/domain/order"
)

func waitCount(t *testing.T, c *atomic.Int64, want int64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for c.Load() != want {
		if time.Now().After(deadline) {
			t.Fatalf("count = %d, want %d", c.Load(), want)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestPumpForwardsOrderEventsAndDetectsReconnect(t *testing.T) {
	t.Parallel()

	var reconnects atomic.Int64
	var connected atomic.Bool
	s := &Streamer{
		ex:          &Exchange{id: "bybit"},
		onReconnect: func() { reconnects.Add(1) },
		pollEvery:   5 * time.Millisecond,
	}

	data := make(chan gctstream.Payload, 4)
	out := make(chan order.Event, 4)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.pump(ctx, connected.Load, data, out)
	}()

	// An order detail forwards; non-order payloads do not.
	data <- gctstream.Payload{Data: "ticker noise"}
	data <- gctstream.Payload{Data: detail(t, nil)}
	select {
	case ev := <-out:
		if ev.Ref.VenueOrderID != "v-1" || ev.Status != order.StatusPartiallyFilled {
			t.Fatalf("event = %+v", ev)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no event forwarded")
	}

	// The pump started disconnected; coming up counts as a reconnect,
	// staying up does not, and every later drop-and-return counts again.
	connected.Store(true)
	waitCount(t, &reconnects, 1)
	time.Sleep(10 * s.pollEvery)
	if got := reconnects.Load(); got != 1 {
		t.Fatalf("stable connection fired onReconnect: %d", got)
	}
	connected.Store(false)
	time.Sleep(10 * s.pollEvery)
	connected.Store(true)
	waitCount(t, &reconnects, 2)

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("pump did not stop on cancel")
	}
	if _, open := <-out; open {
		t.Fatal("out channel not closed after cancel")
	}
}

func TestPumpStopsWhenRelayCloses(t *testing.T) {
	t.Parallel()

	s := &Streamer{ex: &Exchange{id: "bybit"}, onReconnect: func() {}, pollEvery: time.Minute}
	data := make(chan gctstream.Payload)
	out := make(chan order.Event, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.pump(context.Background(), func() bool { return true }, data, out)
	}()

	close(data)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("pump did not stop on closed relay channel")
	}
	if _, open := <-out; open {
		t.Fatal("out channel not closed after relay close")
	}
}
