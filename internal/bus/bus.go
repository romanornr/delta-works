// Package bus is the in-process event bus (ADR-0005). The interface is
// deliberately NATS-shaped so a NATS/JetStream adapter can replace the
// in-process implementation without touching publishers or subscribers.
// Delivery is at-most-once: anything that must not be lost belongs in
// Postgres, not on the bus.
package bus

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Event is a published message. Subject uses dot-separated tokens,
// e.g. "snapshot.taken".
type Event struct {
	Subject string
	At      time.Time
	Payload any
}

// Handler consumes events. It runs on the subscriber's own goroutine and
// must not block for long — slow handlers cause drops for that subscriber.
type Handler func(ctx context.Context, e Event)

// Bus publishes and subscribes to events.
type Bus interface {
	// Publish never blocks on slow subscribers; their events are dropped
	// and counted instead.
	Publish(ctx context.Context, e Event) error
	// Subscribe registers a handler for all subjects with the given prefix
	// ("" subscribes to everything). The returned function unsubscribes.
	Subscribe(subjectPrefix string, h Handler) (unsubscribe func(), err error)
}

const subscriberBuffer = 64

// InProc is the single-process Bus implementation.
type InProc struct {
	mu      sync.Mutex
	nextID  int
	subs    map[int]*subscriber
	baseCtx context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	dropped atomic.Uint64
	closed  bool
}

type subscriber struct {
	prefix string
	ch     chan Event
}

// NewInProc creates a running in-process bus. Close releases its goroutines.
func NewInProc() *InProc {
	ctx, cancel := context.WithCancel(context.Background())
	return &InProc{subs: map[int]*subscriber{}, baseCtx: ctx, cancel: cancel}
}

// Publish implements Bus.
func (b *InProc) Publish(_ context.Context, e Event) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	for _, s := range b.subs {
		if !strings.HasPrefix(e.Subject, s.prefix) {
			continue
		}
		select {
		case s.ch <- e:
		default:
			b.dropped.Add(1)
		}
	}
	return nil
}

// Subscribe implements Bus.
func (b *InProc) Subscribe(subjectPrefix string, h Handler) (func(), error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := b.nextID
	b.nextID++
	s := &subscriber{prefix: subjectPrefix, ch: make(chan Event, subscriberBuffer)}
	b.subs[id] = s

	b.wg.Go(func() {
		for {
			select {
			case <-b.baseCtx.Done():
				return
			case e, ok := <-s.ch:
				if !ok {
					return
				}
				h(b.baseCtx, e)
			}
		}
	})

	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if sub, ok := b.subs[id]; ok {
			delete(b.subs, id)
			close(sub.ch)
		}
	}, nil
}

// Dropped reports how many events were discarded due to slow subscribers.
func (b *InProc) Dropped() uint64 { return b.dropped.Load() }

// Close stops all subscriber goroutines and waits for them to exit.
func (b *InProc) Close() {
	b.mu.Lock()
	b.closed = true
	b.mu.Unlock()
	b.cancel()
	b.wg.Wait()
}
