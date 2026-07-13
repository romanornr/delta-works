# 0005: In-process event bus now, NATS/JetStream later

**Status:** accepted (2026-07-02)

## Background: why an event bus exists at all

Components of this daemon need to react to each other. When a balance snapshot is taken, the API's event stream should show it. When a private websocket reconnects (M2), the reconciliation loop should run immediately instead of waiting for its next timer tick. The naive way to get this is direct calls: the snapshot service calls the API server, the stream adapter calls the reconciler. That couples every producer to every consumer; adding a consumer means editing the producer, and circular imports appear fast.

A publish/subscribe bus inverts this. Producers publish an event under a subject string (`snapshot.taken`, `order.filled`, `stream.reconnected`) without knowing who listens. Consumers subscribe to subjects without knowing who publishes. The two sides only share the bus interface and the subject names.

The real design question was not whether to have a bus, but which one. NATS (with its JetStream persistence layer) is the intended messaging system for the platform's future: it provides durable streams, work queues, and cross-process fan-out. But M1 and M2 are a single process. Running a NATS server next to a single-process daemon means operating, monitoring, securing, and upgrading a piece of infrastructure whose defining features (durability across restarts, delivery between processes) have zero consumers.

## Decision

`internal/bus` defines a small interface shaped like NATS on purpose:

```go
Publish(ctx, Event) error                                  // Event: Subject string, At time, Payload any
Subscribe(subjectPrefix string, handler Handler) (unsubscribe func(), err error)
```

Subjects are dot-separated strings (`order.filled`), and subscription is by prefix, mirroring how NATS subject filtering is used here. The M1 implementation is in-process: a map of subscribers, each with its own goroutine and a bounded channel of 64 events.

The one behavioral decision that everything downstream depends on: **publishing never blocks.** If a subscriber's buffer is full because it is slow, the event is dropped for that subscriber and the `bus_dropped_total` counter is incremented. The alternative (block the publisher until the subscriber catches up) would let one slow consumer stall the trading hot path, which is a worse failure than a missed notification.

The consequence is stated as a contract: delivery is **at-most-once**. Any data that must not be lost does not travel as a bus payload; it is written to Postgres, and the bus event is merely a hint that something changed (ADR-0004 makes Postgres the truth; ADR-0008 builds the guaranteed path from Postgres commits onto the bus). A consumer that misses a hint catches up on its next read or its next timer tick. Every M2 consumer is built to this rule: the reconciler treats a missed `stream.reconnected` as "the next 30-second pass will cover it".

What using it looks like, publisher and subscriber:

```go
// publisher (the streamer wiring): fire the hint, never check who listens
_ = bus.Publish(ctx, bus.Event{
    Subject: "stream.reconnected",
    At:      clk.Now(),
    Payload: venueID,
})

// subscriber (the reconciler): react, but never depend on delivery
unsubscribe, _ := bus.Subscribe("stream.reconnected", func(ctx context.Context, e bus.Event) {
    if venue, ok := e.Payload.(instrument.VenueID); ok {
        kick(venue) // run a pass now instead of waiting for the timer
    }
})
defer unsubscribe()
```

And what a drop looks like end to end, because the contract only clicks once you trace one: the reconciler is mid-pass and slow, its 64-slot buffer is full, a reconnect hint arrives, the bus increments `bus_dropped_total` and moves on, the publisher never blocks. The reconciler finishes its pass, learns nothing, and 30 seconds later its timer fires and the pass it runs covers the same gap the hint would have. The hint was an optimization; correctness never lived in it. Designing consumers so that sentence stays true is the whole discipline this ADR imposes.

## Why shape the interface like NATS from day one

Because migration cost is decided at interface-design time, not migration time. When a second process appears (a separate UI backend, a distributed bot runner), swapping the in-process implementation for a NATS-backed one is an adapter change: same interface, same subjects mapping one-to-one to NATS subjects, no publisher or subscriber edits. If the interface had been designed around in-process convenience (passing pointers, synchronous handlers with return values), every consumer would need rework at exactly the moment the system is becoming more complex anyway.

## Recorded for the future NATS evaluation

The owner is new to NATS, so the candidate uses are written down now for when the time comes:

| Future need | NATS/JetStream feature |
|---|---|
| durable order/fill event streams surviving restarts | JetStream streams, fed by the existing outbox relay (ADR-0008); the outbox stays, only its sink changes |
| distributing execution-algo child orders to workers | work queues (queue-subscribed consumers) |
| shared runtime state across processes | JetStream KV buckets |
| pushing live events to a web UI | fan-out subscriptions bridged to websockets |

## Consequences

- Zero messaging infrastructure to operate until a second process exists.
- At-most-once delivery is a documented constraint on every bus consumer, not a surprise. Code review for a new consumer starts with the question "what happens if you miss one".
- The dropped-event counter makes slow consumers visible in metrics instead of silent.
- The NATS migration, when it happens, is bounded: implement the interface over a NATS connection, deploy the server, switch construction in one place.
