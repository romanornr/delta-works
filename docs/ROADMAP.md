# Roadmap

The long-term goal is a serious, multi-exchange trading platform — not a portfolio tracker. Milestones build on each other; later items are recorded now so early design decisions don't paint them into a corner.

## M1 — Foundation + read-only exchange (current)

Tooling-first skeleton (lint/CI/compose), core runtime (config, logging, DI, metrics, in-proc bus), pure domain layer, GCT adapter behind ports with rate-limit + breaker, Postgres checkpoints + QuestDB time-series, portfolio snapshot daemon. Spec: [specs/m1-foundation.md](specs/m1-foundation.md).

## M2 — Order management

Pure order state machine (every transition persisted: orders/fills tables), our ULID client order IDs as idempotency keys, private order-event streaming with reconnect, reconciliation loop diffing venue open orders vs local state, per-bot ledger with lots, Postgres outbox → bus.

## M3 — Grid bots

Multiple concurrent grid bots, each an actor goroutine controlled via mailbox commands: pause / resume / stop individually. Live-adjustable lower/upper limits as a validated transaction (new ladder diffed against open orders and lots; buy side checked against free quote cash, sell side against inventory qty, reservations prevent double-claiming) — cancel/replace only the delta so PNL history is untouched. **PNL attribution is exact by construction:** sell fills pair with the lot bought one grid level below (grid pairing, not FIFO) → realized grid-cycle profit; unmatched lots are inventory → price-movement PNL = mark-to-market vs lot cost.

## M4+ — Execution & cross-venue

- Cross-exchange arbitrage (a strategy holding two registry entries).
- Execution algos as parent/child slicers feeding the OMS: TWAP, VWAP, iceberg, adaptive/dark-ice, pegged, scaling, pair trades, delta hedge.
- NATS/JetStream replaces the in-proc bus when multi-process (ADR-0005).
- Native exchange adapters (coder/websocket) replacing GCT per venue where it matters (ADR-0003).
- Web UI / TUI over a proper HTTP API.

## Later — Quant analytics (sell-side style)

Nomura/GS/BofA-grade analytics computed over QuestDB series: standard deviations, z-scores, sigma-event detection, drawdown, cumulative net flow, upside skew, bell-curve/distribution charts, vol expansion from hedging. Implication for today: ingest time-series richly (balances, tickers, later fills, marks, funding) with clean symbols and designated timestamps so these are pure read-side computations later.

## Someday

Market making.
