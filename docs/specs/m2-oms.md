# Spec: M2 — Order management

**Status:** accepted 2026-07-05, implementation in progress.

## Goal

An order management core where every state transition is persisted: a pure order state machine, ULID client order IDs as idempotency keys, private order-event streaming with adapter-owned reconnect, a reconciliation loop diffing venue state against local state, a per-bot ledger with lots (M3-ready via a selector seam), and a Postgres outbox feeding the bus (ADR-0008). Orders enter the system only through the control plane: `OrderService` RPCs and their `deltactl order` commands (ADR-0007) — bots become a second caller in M3.

## Package layout

Additions to the M1 tree:

```
internal/id/                # ULID generation (oklog/ulid/v2, crypto/rand entropy)
internal/domain/order/      # state machine: Transition(current, event) (decision, error)  [pure]
internal/domain/ledger/     # Lot, Closure, LotSelector interface, FIFO selector           [pure]
internal/service/order/     # place/cancel/apply-event orchestration
internal/service/reconcile/ # periodic venue-vs-local diff loop
internal/service/outbox/    # outbox relay: poll → bus.Publish
internal/adapters/gct/      # gains OrderPlacer + PrivateStreamer implementations
internal/adapters/postgres/ # gains OrderStore, OutboxStore, LedgerStore
proto/control/v1/           # gains orders.proto (OrderService) + new Event oneof arms
```

Domain packages stay pure: `internal/id` owns the ULID dependency; domain code treats IDs as opaque strings.

## Key behaviors

### Order state machine

States are unchanged from M1's `ports` seam: `pending`, `open`, `partially_filled`, `filled`, `canceled`, `rejected`, `expired`. Terminal states: `filled`, `canceled`, `rejected`, `expired`. Each state has a rank — `pending`(0) < `open`(1) < `partially_filled`(2) < terminal(3) — used as a monotonic guard against stale events.

`order.Event` carries the venue's view: status, cumulative filled qty, and per-fill facts (`VenueFillID`, price, `Fee`, `FeeCurrency`) for exact fill dedupe and lot cost basis.

Transition table (event status × stored status → decision):

| stored \ event | pending | open | partially_filled | filled | canceled | rejected | expired |
|---|---|---|---|---|---|---|---|
| pending | drop | apply | apply | apply | apply | apply | apply |
| open | drop | drop* | apply | apply | apply | apply | apply |
| partially_filled | drop | drop* | apply* | apply | apply | apply | apply |
| terminal | drop | drop | drop* | drop* | drop* | drop* | drop* |

`*` = fills still extracted: any event whose cumulative filled qty exceeds the stored value records the delta as a fill, even when the status itself is dropped or a same-rank repeat. The table is total: every (stored, event) pair is either an apply row or covered by the drop rule (rank-regressing or same-rank event with no new cumulative fill → drop, counted in `order_events_dropped_total`).

Apply is idempotent and runs in one transaction with `SELECT ... FOR UPDATE` on the order row:

1. Fill delta = event cumulative filled qty − stored filled qty. Delta > 0 inserts a `fills` row (deduped by `venue_fill_id` where the venue provides one). A negative delta never un-fills. From a same- or lower-rank event it is ordinary stale traffic, dropped by the rank rule. From a rank-advancing event the status transition still applies — venue wins on state — while the fill regression is rejected and counted per source: `order_events_dropped_total{reason="negative_fill_delta"}` on the ack/stream path, `reconcile_diffs_total{kind="fill_anomaly"}` when reconciliation discovers it.
2. Status change appends an `order_transitions` row (`seq` increments per order) and updates the order row.
3. Ledger postings (below) happen in the same transaction.
4. Outbox rows for `order.updated` / `order.filled` are inserted in the same transaction (ADR-0008).

Cancel is an intent, not a state: `CancelOrder` stamps `cancel_requested_at` and calls the venue; the `canceled` transition arrives like any other venue event (stream or reconcile).

### Client order IDs (ULID)

`orders.client_order_id` is the primary key and the idempotency key sent to the venue. The pending row is inserted **before** `PlaceOrder` is called, so a crash mid-submit leaves a visible pending order rather than an untracked venue order. Submit failure handling:

- Ambiguous failure (timeout, 5xx): retry with the **same** ULID — never regenerate. A duplicate-client-order-id error from the venue means the original submit landed: treat as success. The error response carries no `venue_order_id`; the order stays `pending` until the next stream event or reconcile pass supplies it, matched on the client order ID, which is sent on every submit and echoed back by the venue.
- Retries exhausted: the order stays `pending`. Reconciliation either adopts it (found on the venue) or marks it `rejected` (reason `submit-lost`) after a grace window of 2× the reconcile interval.

### Private order-event streaming

The GCT adapter implements `ports.PrivateStreamer`: it owns the websocket lifecycle including reconnect, and publishes `stream.reconnected` on the bus after every reconnect so reconciliation can close the gap. Stream events feed `ApplyEvent` with `source=stream`; the synchronous PlaceOrder/CancelOrder response feeds it with `source=ack`. Ordering between the two is irrelevant — the monotonic guard and cumulative-fill delta make apply order-independent.

### Reconciliation

`ports.OrderPlacer` reconciliation surface: `OpenOrders(ctx)` is venue-wide (no symbol filter — the loop must see everything), plus `GetOrder(ctx, order.Ref)` for point lookups. No implementations existed before M2, so the signature change is free.

The loop runs per venue every 30s (config) and immediately on `stream.reconnected`. Venue wins on execution facts:

- Local non-terminal order absent from venue open orders → `GetOrder` → apply its terminal transition (`source=reconcile`).
- `filled_qty` drift → apply the venue value through the normal fill-delta path. Reconcile-sourced fills carry no `venue_fill_id` or fee and are priced at the venue's average fill price — an accepted approximation over losing the quantity entirely, and identifiable later via `source=reconcile` on the transition.
- Venue order unknown locally → **not** adopted; publish `reconcile.orphan` and raise a gauge. Orphans are a human problem (foreign order or lost state), not something to auto-import.

### Ledger

Per-bot inventory as lots, posted inside the same `ApplyEvent` transaction as the fill:

- Buy fill opens a `Lot` (qty, cost price from the fill).
- Sell fill closes lots chosen by the `LotSelector` port; M2 ships FIFO. M3's grid pairing is just another selector — the seam is the point.
- Oversell (sell fill exceeds open lot qty): close what matches, record the unmatched remainder in `unmatched_sells`, publish `ledger.unmatched_sell` and count it. Never hard-fail — a venue-reported fill is a fact; M3 reservations prevent oversell upfront. Unmatched remainders are not retro-matched when later buy fills arrive; they stay recorded for operator attention.
- `Request` carries `BotID` from day one; RPC-placed orders use the reserved `bot_id` `manual`.

Ledger posting is serialized with a transaction-scoped advisory lock per `(bot_id, venue, base, quote)` inventory key. Matching order is the serial order in which `ApplyEvent` acquires this lock; FIFO applies by `(opened_at, id)` among lots already posted. Event-time FIFO under arbitrary cross-order arrival is not promised because it cannot coexist with no retro-matching.

M2 lot cost basis is execution-price-only: fees are recorded on fills but not allocated to lots in M2; fee-aware inventory and PnL are deferred — a known, intentional accuracy limitation. Adapter code enforces the cross-table invariant: lots open only from buy fills, and closures and unmatched rows come only from sell fills of the same bot and instrument.

### Outbox

Order events reach the bus only through the outbox — services never `bus.Publish` them directly (ADR-0008). The relay is a single goroutine polling at 200ms–1s (config): `SELECT ... FROM outbox WHERE published_at IS NULL ORDER BY id LIMIT 100 FOR UPDATE SKIP LOCKED`, publish each to the bus, stamp `published_at`. Delivery is at-least-once into the bus; the bus stays at-most-once to subscribers — Postgres is the truth either way (ADR-0004). Published rows are deleted after 7 days.

### Metrics

`order_events_dropped_total{venue,reason}`, `outbox_published_total`, `outbox_unpublished_rows`, `outbox_oldest_unpublished_age_seconds`, `reconcile_diffs_total{venue,kind}`, `reconcile_duration_seconds{venue}`, `reconcile_last_success_timestamp_seconds{venue}` (staleness-alertable, like M1's snapshot metric), `ledger_unmatched_sells_total{venue}`.

## Storage

Migrations `0002_orders`, `0003_outbox`, `0004_ledger` (goose, embedded, brand-neutral names). All money columns are `numeric`.

- `orders`: `client_order_id` text PK, venue, base, quote, venue_symbol, side, type, price, qty, filled_qty, avg_fill_price, status, venue_order_id, bot_id, cancel_requested_at, reason, created_at, updated_at. Partial `UNIQUE(venue, venue_order_id)` where venue_order_id is set; indexes `(venue, status)` and `(bot_id, created_at DESC)`.
- `order_transitions`: identity PK, client_order_id FK, seq with `UNIQUE(client_order_id, seq)`, from_status, to_status, cumulative filled_qty, source `CHECK (source IN ('local','stream','ack','reconcile'))`, reason, occurred_at, recorded_at.
- `fills`: identity PK, client_order_id + transition FKs, qty (delta), price, fee, fee_currency, venue_fill_id (partial unique), occurred_at.
- `outbox`: identity PK, subject, payload jsonb, created_at, published_at NULL; partial index on unpublished rows.
- `lots`: ULID text PK, bot_id, venue, base, quote, qty, remaining_qty, cost_price, opened_by_fill_id bigint unique FK, status, opened_at, closed_at.
- `lot_closures`: identity PK, lot_id text FK, sell_fill_id bigint FK, qty, price, closed_at, `UNIQUE(lot_id, sell_fill_id)`.
- `unmatched_sells`: sell_fill_id bigint PK/FK, bot_id, venue, base, quote, qty, occurred_at.
- QuestDB gains a `fills` series (sym: venue, symbol, side, bot; dbl: qty, price, fee) — analytics only, ADR-0004.

## Control plane

- `proto/control/v1/orders.proto`: `OrderService` with `PlaceOrder`, `CancelOrder`, `ListOrders`; protovalidate rules; decimals as strings (matching existing conventions). Wire package stays `control.v1`.
- Existing `Event` oneof gains append-only arms: `order_updated = 11`, `order_filled = 12`, `reconcile_diff = 13`.
- `deltactl order place|cancel|list` speak these RPCs — the only way to place an order until M3 (ADR-0007).

## Verification

1. Exhaustive transition-table unit test: every (stored, event) status pair asserts the decision above.
2. `make test-integration`: ApplyEvent idempotency (same event twice → one transition, one fill), out-of-order convergence (fill before ack → same final state), outbox round-trip ordering, reconcile against a fake venue (terminal adoption, drift, orphan), FIFO lot math including oversell.
3. Live testnet checklist: place → open; cancel → canceled; fill → lot opened/closed; kill the stream → reconcile converges; restart the daemon mid-flight → no duplicate orders, outbox drains.
4. `make ci` green throughout.

## Delivery slicing

Nine PRs, each independently green: (1) this spec + ADR-0008; (2) `internal/id` + `domain/order` state machine; (3) migrations 0002/0003 + `OrderStore.ApplyEvent` + `OutboxStore`; (4) outbox relay service; (5) GCT `OrderPlacer` + `PrivateStreamer`; (6) `service/order`; (7) `service/reconcile`; (8) `domain/ledger` + migration 0004; (9) proto + `OrderService` + `deltactl order` commands + live verification.
