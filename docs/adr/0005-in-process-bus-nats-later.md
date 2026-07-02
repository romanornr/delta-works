# 0005 — In-process event bus now, NATS/JetStream later

**Status:** accepted (2026-07-02)

## Context

Services need to publish events (snapshot taken; later: order transitions, fills, bot state changes) without coupling to consumers. NATS/JetStream is the intended durable messaging layer eventually, but M1 is a single process and standing up NATS now adds operational surface without need.

## Decision

`internal/bus` defines a deliberately NATS-shaped interface — subject strings, `Publish(ctx, Event)`, `Subscribe(subjectPrefix, handler)` — implemented in-process with bounded per-subscriber channels. Publishing never blocks the hot path: on a full subscriber buffer the event is dropped and `bus_dropped_total` incremented. Delivery is at-most-once; anything that must not be lost goes to Postgres, not the bus.

## Consequences

- Swapping in NATS later is an adapter change, not a service change; subjects map 1:1.
- Candidate NATS/JetStream uses when it arrives (recorded for future evaluation, owner is new to NATS): durable event streams for order/fill transitions (with a Postgres outbox for exactly-once-ish publishing), work queues for execution-algo child orders, KV for shared runtime state across processes, and fan-out to a future UI over websockets.
- Until then, at-most-once semantics are a documented constraint on bus consumers.
