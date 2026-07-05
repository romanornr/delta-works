# 0008 — Transactional outbox for order events; state row plus transition log

**Status:** accepted (2026-07-05)

## Context

The in-process bus is at-most-once and may drop events under backpressure (ADR-0005). That is acceptable for snapshots, but order transitions and fills drive the ledger and future consumers: losing one silently corrupts derived state. Postgres is the system of record (ADR-0004), so the durability question is how bus publishing relates to the database commit, and how much of the order's history the database keeps.

## Decision

**Outbox.** Every service that records an order event writes a row to an `outbox` table (subject + jsonb payload) in the same transaction as the state change; services never publish order events to the bus directly. A single relay goroutine polls at 200ms–1s: `SELECT ... FROM outbox WHERE published_at IS NULL ORDER BY id LIMIT 100 FOR UPDATE SKIP LOCKED`, publishes each row to the bus, stamps `published_at`, and deletes published rows after 7 days. Publishing is an in-process call that never blocks (ADR-0005), so holding the row locks across it is safe. Delivery into the bus is at-least-once; the bus remains at-most-once to subscribers, and anything that must be exact reads Postgres.

Rejected: publish-after-commit (a dual write — a crash between commit and publish loses the event, which is the exact failure this exists to prevent); LISTEN/NOTIFY to wake the relay (saves at most one poll interval of latency at the cost of a second delivery mechanism to operate; revisit if the poll ever matters). Payloads are jsonb rather than proto bytes so rows are readable in psql when debugging; the bus is in-process, so there is no wire-format concern.

**State shape.** Orders are stored as a current-state row plus an append-only `order_transitions` log written in the same transaction. Full event sourcing (state derived by replaying events) was rejected: it adds replay and projection machinery with no consumer that needs it, while the transition log already gives a complete audit trail and the state row gives cheap queries.

## Consequences

- Order events reach subscribers with up to one poll interval of latency; fine for reconciliation triggers and UI, and anything tighter should read Postgres anyway.
- Unpublished-row count and oldest-row age are exported as metrics, so a stuck relay is alertable.
- When NATS/JetStream arrives (ADR-0005), it replaces the relay's sink, not the outbox — the same table then feeds a durable stream. A networked sink also changes the locking shape (claim rows, release the transaction, publish, then mark), still a relay-only change.
