package bus

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestPublishSubscribePrefix(t *testing.T) {
	b := NewInProc()
	defer b.Close()

	var mu sync.Mutex
	var got []string
	done := make(chan struct{}, 2)

	unsub, err := b.Subscribe("snapshot.", func(_ context.Context, e Event) {
		mu.Lock()
		got = append(got, e.Subject)
		mu.Unlock()
		done <- struct{}{}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer unsub()

	for _, subject := range []string{"snapshot.taken", "order.filled", "snapshot.failed"} {
		if err := b.Publish(context.Background(), Event{Subject: subject, At: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}

	for range 2 {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for events")
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 || got[0] != "snapshot.taken" || got[1] != "snapshot.failed" {
		t.Errorf("got %v, want [snapshot.taken snapshot.failed]", got)
	}
}

func TestSlowSubscriberDropsInsteadOfBlocking(t *testing.T) {
	b := NewInProc()
	defer b.Close()

	block := make(chan struct{})
	_, err := b.Subscribe("", func(_ context.Context, _ Event) { <-block })
	if err != nil {
		t.Fatal(err)
	}

	// Fill the handler (1 in-flight) + buffer, then overflow.
	for range subscriberBuffer + 10 {
		_ = b.Publish(context.Background(), Event{Subject: "x"})
	}

	if b.Dropped() == 0 {
		t.Error("expected dropped events for a blocked subscriber")
	}
	close(block)
}

func TestUnsubscribeStopsDelivery(t *testing.T) {
	b := NewInProc()
	defer b.Close()

	delivered := make(chan Event, 1)
	unsub, err := b.Subscribe("", func(_ context.Context, e Event) { delivered <- e })
	if err != nil {
		t.Fatal(err)
	}
	unsub()

	_ = b.Publish(context.Background(), Event{Subject: "x"})
	select {
	case <-delivered:
		t.Error("received event after unsubscribe")
	case <-time.After(50 * time.Millisecond):
	}
}
