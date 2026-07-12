# Spec: M2: Order management

**Status:** accepted 2026-07-05, implementation in progress. This document is normative: implementations follow it, and changes to behavior change this file in the same PR.

## What M2 builds, and the one idea underneath all of it

M2 is the order management core: the machinery that places orders on exchanges, tracks what happens to them, and keeps an exact account of the inventory the fills produce. Concretely: a pure order state machine, ULID client order IDs used as idempotency keys, a private websocket stream of order events per venue, a reconciliation loop that periodically compares venue state against local state, a per-bot inventory ledger built from lots, and a Postgres outbox that delivers order events to the rest of the system without losing any (ADR-0008). Orders enter the system only through the control plane: `OrderService` RPCs and their `deltactl order` commands (ADR-0007). Bots become a second caller in M3.

The one idea underneath everything: **the exchange is the authority on what happened, and our job is to converge on its view without ever losing or double-counting a fact.** Exchange APIs deliver events late, out of order, more than once, and occasionally contradictorily. Every design below (ranks, cumulative quantities, idempotent apply, reconciliation) is a consequence of taking that seriously instead of assuming a well-behaved event stream.

## Package layout

Additions to the M1 tree:

```
internal/id/                # ULID generation (oklog/ulid/v2, crypto/rand entropy)
internal/domain/order/      # state machine: Transition(current, event) (decision, error)  [pure]
internal/domain/ledger/     # Lot, Closure, LotSelector interface, FIFO selector           [pure]
internal/service/order/     # place/cancel/apply-event orchestration
internal/service/reconcile/ # periodic venue-vs-local diff loop
internal/service/outbox/    # outbox relay: poll, then bus.Publish
internal/adapters/gct/      # gains OrderPlacer + PrivateStreamer implementations
internal/adapters/postgres/ # gains OrderStore, OutboxStore, ledger posting
proto/control/v1/           # gains orders.proto (OrderService) + new Event oneof arms
```

Domain packages stay pure (ADR-0002): `internal/id` owns the ULID dependency; domain code treats IDs as opaque strings.

## Key behaviors

### The order state machine

An order moves through seven states. Four of them are terminal, meaning no further state change is possible:

| State | Meaning | Terminal |
|---|---|---|
| `pending` | we created it locally; the venue may not know it yet | no |
| `open` | the venue accepted it; nothing executed yet | no |
| `partially_filled` | some quantity executed | no |
| `filled` | fully executed | yes |
| `canceled` | canceled before completion | yes |
| `rejected` | the venue refused it | yes |
| `expired` | the venue timed it out | yes |

Each state has a rank: `pending`(0) < `open`(1) < `partially_filled`(2) < terminal(3). The rank is a monotonic guard: an order's rank never decreases. This is how out-of-order delivery is tamed. If a `filled` event arrives and then a stale `open` event from earlier arrives late, the `open` is rank-regressing and its status is dropped; without ranks, the late event would resurrect a finished order.

The second taming device is that `order.Event` carries the **cumulative** filled quantity, not a per-fill amount. The fill delta is computed locally as event cumulative minus stored cumulative. This makes applying events order-independent for quantities: whichever of two events arrives first, the running total converges to the same value, and a duplicate event produces a delta of zero. Events also carry per-fill facts where the venue provides them (`VenueFillID`, price, `Fee`, `FeeCurrency`) for exact fill deduplication and ledger cost basis.

The full decision table (event status horizontally, stored status vertically):

| stored \ event | pending | open | partially_filled | filled | canceled | rejected | expired |
|---|---|---|---|---|---|---|---|
| pending | drop | apply | apply | apply | apply | apply | apply |
| open | drop | drop* | apply | apply | apply | apply | apply |
| partially_filled | drop | drop* | apply* | apply | apply | apply | apply |
| terminal | drop | drop | drop* | drop* | drop* | drop* | drop* |

`*` means fills are still extracted: any event whose cumulative filled quantity exceeds the stored value records the delta as a fill, even when the status itself is dropped as stale or is a same-rank repeat. Why: a post-terminal `partially_filled` event that carries a higher cumulative than we recorded is stale as a *status* but its quantity is an execution fact we would otherwise lose. Post-terminal `canceled`/`rejected`/`expired` events get the same treatment because venues report cancel-after-partial-fill with the final cumulative attached.

The table is total: every (stored, event) pair is either an apply cell or covered by the drop rule (rank-regressing, or same-rank with no new cumulative fill). Drops are counted in `order_events_dropped_total`, never silent. An exhaustive unit test enumerates every pair.

Applying an event is idempotent and runs in one transaction with `SELECT ... FOR UPDATE` on the order row:

1. Fill delta = event cumulative minus stored cumulative. A positive delta inserts a `fills` row (deduplicated by `venue_fill_id` where the venue provides one). A negative delta never un-fills. From a same- or lower-rank event a negative delta is ordinary stale traffic, dropped by the rank rule. From a rank-advancing event the status transition still applies (venue wins on state) while the fill regression is rejected and counted per source: `order_events_dropped_total{reason="negative_fill_delta"}` on the ack/stream path, `reconcile_diffs_total{kind="fill_anomaly"}` when reconciliation discovers it. The rank-advancing case is not hypothetical: several venues send `canceled` with executed quantity 0 even after partial fills, and dropping the whole event would leave the order stuck non-terminal forever (this exact bug was found by a property test and fixed).
2. A status change appends an `order_transitions` row (`seq` increments per order) and updates the order row. The transition log is the audit trail: which events arrived, from which source, in what order, and what each one did.
3. Ledger postings (below) happen in the same transaction.
4. Outbox rows for `order.updated` / `order.filled` are inserted in the same transaction (ADR-0008).

Cancel is an intent, not a state: `CancelOrder` stamps `cancel_requested_at` and asks the venue; the actual `canceled` transition arrives later like any other venue event (stream or reconcile). Treating the request as the state would mean recording something the venue has not confirmed.

### Client order IDs

Every order gets a ULID (a 26-character sortable unique ID) generated by us before anything is sent to the venue. `orders.client_order_id` is the primary key locally and travels to the venue with the order, where it functions as the **idempotency key**: submitting twice with the same ID cannot create two orders.

The pending row is inserted **before** `PlaceOrder` is called. Order of operations matters here: if the process crashes mid-submit, the failure mode is a visible local `pending` order that reconciliation will resolve, instead of an untracked live order on the venue. Between "money exists that we do not know about" and "a record exists for money that may not", the second is always the recoverable one.

Submit failure handling:

- **Ambiguous failure** (timeout, 5xx: the venue may or may not have accepted the order): retry with the **same** ULID, never regenerate. Regenerating would turn one intended order into possibly two live ones. If the venue answers "duplicate client order ID", that is good news: the original submit landed, treat it as success. That error carries no `venue_order_id`, so the order stays `pending` until the next stream event or reconcile pass supplies one, matched on the client order ID the venue echoes back.
- **Retries exhausted**: the order stays `pending` and the caller is told the submit is unsettled. Reconciliation then either adopts the order (found on the venue) or marks it `rejected` with reason `submit-lost` after a grace window of twice the reconcile interval. The grace window exists because a submit can succeed after our last retry timed out.

### Private order-event streaming

The GCT adapter implements `ports.PrivateStreamer`: it owns the authenticated websocket lifecycle including reconnects, and publishes `stream.reconnected` on the bus after every reconnect so reconciliation can immediately close whatever gap the disconnection opened. Stream events feed `ApplyEvent` with `source=stream`; the synchronous PlaceOrder/CancelOrder response feeds it with `source=ack`. The two race freely and the ordering between them is irrelevant by construction: the rank guard and cumulative-fill delta make apply order-independent.

### Reconciliation

Streams miss events: sockets drop, processes restart, venues have gaps. Reconciliation is the backstop that guarantees convergence anyway. The `ports.OrderPlacer` surface for it: `OpenOrders(ctx)` is venue-wide with no symbol filter, because the loop must see everything including orders it does not expect, plus `GetOrder(ctx, order.Ref)` for point lookups.

The loop runs per venue every 30s (configurable) and immediately on `stream.reconnected`. The governing rule is that the venue wins on execution facts:

| Situation | Action |
|---|---|
| local non-terminal order absent from venue open orders | `GetOrder` point lookup, apply its terminal transition (`source=reconcile`) |
| `filled_qty` drift between local and venue | apply the venue value through the normal fill-delta path |
| venue order unknown locally | **not** adopted; publish `reconcile.orphan` and raise a gauge |

Reconcile-sourced fills carry no `venue_fill_id` or fee and are priced at the venue's average fill price: an accepted approximation, preferred over losing the quantity entirely, and identifiable forever via `source=reconcile` on the transition row. Orphans are never auto-imported because an unknown live order means either a foreign order on a shared account or local state loss, and both need a human, not a guess.

### Ledger

The ledger answers "what does each bot hold and what did it pay", using lot accounting: each buy fill opens a `Lot` (quantity, cost price); each sell fill closes lots chosen by the `LotSelector` interface. M2 ships FIFO; M3's grid pairing is just another selector, and that seam is the reason the interface exists. Posting happens inside the same `ApplyEvent` transaction as the fill, so fills and their inventory effects commit atomically.

- Oversell (a sell fill exceeding open lot quantity): close what matches, record the remainder in `unmatched_sells`, publish `ledger.unmatched_sell`, count it. Never hard-fail: a venue-reported fill is a fact. Unmatched remainders are not retro-matched when later buys arrive; they stay recorded for operator attention. M3 reservations prevent oversell before submission.
- `Request` carries `BotID` from day one; RPC-placed orders use the reserved `bot_id` `manual`.

Ledger posting is serialized with a transaction-scoped advisory lock per `(bot_id, venue, base, quote)` inventory key. The lock exists because row locks cannot lock rows that do not exist yet: without it, a sell processed while a buy for the same inventory is uncommitted sees zero lots and records a false, permanent oversell. Matching order is therefore the serial order in which `ApplyEvent` acquires this lock, with FIFO by `(opened_at, id)` among lots already posted. Event-time FIFO under arbitrary cross-order arrival is explicitly not promised: it cannot coexist with no-retro-matching, and promising it anyway would be documentation lying about the code.

M2 lot cost basis is execution-price-only. Fees are recorded on fills but not allocated to lots, because fees come in three shapes with different inventory meanings (quote-currency fees raise effective cost, base-currency fees reduce received inventory, third-currency fees need an exchange rate), and a single cost price cannot express them without a convention that does not exist yet. This is a known, intentional accuracy limitation, resolved in M3. Adapter code enforces the cross-table invariant that lots open only from buy fills and closures and unmatched rows come only from sell fills of the same bot and instrument.

### Outbox

Order events reach the bus only through the outbox; services never `bus.Publish` them directly (ADR-0008 explains the dual-write problem this prevents). The relay is a single goroutine polling at 200ms to 1s (configurable): `SELECT ... FROM outbox WHERE published_at IS NULL ORDER BY id LIMIT 100 FOR UPDATE SKIP LOCKED`, publish each to the bus, stamp `published_at`. Delivery is at-least-once into the bus; the bus stays at-most-once to subscribers; Postgres is the truth either way (ADR-0004). Published rows are deleted after 7 days.

### Metrics

| Metric | What it tells an operator |
|---|---|
| `order_events_dropped_total{venue,reason}` | volume of stale/duplicate/anomalous venue traffic, by cause |
| `outbox_published_total`, `outbox_unpublished_rows`, `outbox_oldest_unpublished_age_seconds` | relay health; a stuck relay makes the age grow linearly |
| `reconcile_diffs_total{venue,kind}` | how often reconciliation finds and repairs divergence, by kind |
| `reconcile_duration_seconds{venue}` | reconcile pass cost |
| `reconcile_last_success_timestamp_seconds{venue}` | staleness-alertable, same pattern as M1's snapshot metric |
| `ledger_unmatched_sells_total{venue}` | oversells seen on the stream/ack path |

## Storage

Migrations `0002_orders`, `0003_outbox`, `0004_ledger` (goose, embedded, brand-neutral names). All money columns are `numeric` (ADR-0002).

| Table | Purpose | Key columns and constraints |
|---|---|---|
| `orders` | current state, one row per order | `client_order_id` text PK; venue, base, quote, venue_symbol, side, type, price, qty, filled_qty, avg_fill_price, status, venue_order_id, bot_id, cancel_requested_at, reason, created_at, updated_at. Partial `UNIQUE(venue, venue_order_id)` where set; indexes `(venue, status)` and `(bot_id, created_at DESC)` |
| `order_transitions` | append-only audit trail | identity PK, FK to orders, `seq` with `UNIQUE(client_order_id, seq)`, from/to status, cumulative filled_qty, source `CHECK (source IN ('local','stream','ack','reconcile'))`, reason, occurred_at, recorded_at |
| `fills` | one row per fill delta | identity PK, order + transition FKs, qty (delta), price, fee, fee_currency, venue_fill_id (partial unique), occurred_at |
| `outbox` | ADR-0008 event queue | identity PK, subject, payload jsonb, created_at, published_at NULL; partial index on unpublished rows |
| `lots` | open/closed inventory | ULID text PK, bot_id, venue, base, quote, qty, remaining_qty, cost_price, `opened_by_fill_id` bigint unique FK, status, opened_at, closed_at; CHECKs couple status, remaining_qty and closed_at |
| `lot_closures` | which lot a sell consumed | identity PK, lot FK, sell fill FK, qty, price, closed_at, `UNIQUE(lot_id, sell_fill_id)` |
| `unmatched_sells` | oversell remainders | `sell_fill_id` bigint PK/FK, bot_id, venue, base, quote, qty, occurred_at |

QuestDB gains a `fills` series (symbols: venue, symbol, side, bot; doubles: qty, price, fee). Analytics only, per ADR-0004.

## Control plane

- `proto/control/v1/orders.proto`: `OrderService` with `PlaceOrder`, `CancelOrder`, `ListOrders`; protovalidate rules in the schema; decimals cross the wire as strings. The wire package stays `control.v1` (brand-neutral, ADR-0007).
- The existing `Event` oneof gains append-only arms: `order_updated = 11`, `order_filled = 12`, `reconcile_diff = 13`.
- `deltactl order place|cancel|list` speak these RPCs, and they are the only way to place an order until M3 (ADR-0007).

## Verification

1. Exhaustive transition-table unit test: every (stored, event) status pair asserts the decision above, plus property-based tests (rank monotonicity, fill monotonicity, idempotency, permutation invariance) over generated event sequences.
2. `make test-integration`: ApplyEvent idempotency (same event twice produces one transition and one fill), out-of-order convergence (fill before ack reaches the same final state), outbox round-trip ordering, reconcile against a fake venue (terminal adoption, drift, orphan), FIFO lot math including both oversell shapes, and the ledger concurrency cases (racing buy/sell and sell/sell on one inventory key).
3. Live checklist against the real venue: place then observe open; cancel then observe canceled; fill then observe the lot open and close; kill the stream and watch reconcile converge; restart the daemon mid-flight and confirm no duplicate orders and a draining outbox.
4. `make ci` green throughout.

## Delivery slicing

Nine PRs, each independently green:

| # | Content |
|---|---|
| 1 | this spec + ADR-0008 |
| 2 | `internal/id` + `domain/order` state machine |
| 3 | migrations 0002/0003 + `OrderStore.ApplyEvent` + `OutboxStore` |
| 4 | outbox relay service |
| 5 | GCT `OrderPlacer` + `PrivateStreamer` |
| 6 | `service/order` |
| 7 | `service/reconcile` |
| 8 | `domain/ledger` + migration 0004 |
| 9 | proto + `OrderService` + `deltactl order` + full wiring + live verification |
