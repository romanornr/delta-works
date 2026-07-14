# Roadmap

The long-term goal is a serious multi-exchange trading platform, not a portfolio tracker. Milestones are named for the capability the operator gains, they build on each other in the order below, and later items are written down now for one specific reason: early design decisions must not paint them into a corner. Several current architecture choices (the selector interface in the ledger, the NATS-shaped bus, the time-series ingestion discipline) exist because a later milestone on this page needs them.

## Account watch (delivered)

A daemon that snapshots exchange balances into QuestDB every minute and records durable checkpoints in Postgres. Modest by design: the milestone exists to force all the engineering infrastructure into existence (lint and CI, config, logging, dependency injection, migrations, generated SQL, integration tests against real databases, metrics, graceful shutdown, the exchange adapter and its resilience stack) against a problem that cannot lose money. Spec: [specs/account-watch.md](specs/account-watch.md).

## Manual trading (code complete; live verification pending)

The machinery that places orders and accounts for what happens to them. A pure order state machine where every transition is persisted, ULID client order IDs as idempotency keys so retries can never create duplicate orders, private order-event streaming per venue, a reconciliation loop that periodically converges local state with venue state, a per-bot inventory ledger built from lots, and a transactional outbox so no order event is ever lost between the database and the rest of the system. Orders enter only through the typed control-plane API and its `deltactl order` commands. Spec: [specs/manual-trading.md](specs/manual-trading.md).

## Grid bots

A grid bot places a ladder of buy orders below the price and sell orders above it, profiting when the price oscillates across levels. This milestone runs multiple grid bots concurrently, each as an actor goroutine controlled through mailbox commands: pause, resume, stop individually.

Two design problems dominate, and both have their foundations laid in the manual-trading milestone:

- **Live limit adjustment as a validated transaction.** Changing a running bot's price range means computing the new order ladder, diffing it against the open orders and lots that already exist, checking the buy side against free quote cash and the sell side against actual inventory (with reservations so two bots cannot claim the same funds), and then cancelling and replacing only the delta. Replacing only the delta is what keeps profit history intact.
- **Exact profit attribution by construction.** A sell fill pairs with the lot bought one grid level below it. That pairing is just another `LotSelector` implementation (the interface the ledger already ships with FIFO), and it splits profit cleanly in two: realized grid-cycle profit from paired lots, and price-movement profit from unmatched inventory marked to market against lot cost. No estimation, no averaging: every profit number traces to specific fills.

## Execution and cross-venue

- Cross-exchange arbitrage: a strategy holding two venue registry entries and trading the spread between them.
- Execution algorithms as parent/child order slicers feeding the same order management core: TWAP and VWAP (time- and volume-weighted slicing of a large order), iceberg (show a sliver, hide the size), adaptive and pegged variants, scaling entries, pair trades, delta hedging.
- NATS/JetStream replaces the in-process bus when the platform becomes multi-process ([ADR-0005](adr/0005-in-process-bus-nats-later.md) records the migration path and candidate uses).
- Native exchange adapters replace gocryptotrader per venue where speed or reliability justifies it ([ADR-0003](adr/0003-gct-quarantine.md) makes this a drop-in swap).
- A web UI over the same control-plane API the CLI uses, with TypeScript clients generated from the same protobuf contract ([ADR-0007](adr/0007-connectrpc-control-plane.md)).

## Later: quant analytics

Sell-side style analytics computed over the QuestDB series: standard deviations and z-scores, sigma-event detection, drawdown, cumulative net flow, skew, distribution charts, volatility expansion. The implication for today, and the reason this section exists on the roadmap at all: ingest time-series richly (balances, tickers, later fills, marks, funding) with clean symbols and designated timestamps, so all of this becomes pure read-side computation later instead of a re-ingestion project.

## Someday

Market making.
