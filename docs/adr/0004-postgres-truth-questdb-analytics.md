# 0004: Postgres is truth, QuestDB is analytics

**Status:** accepted (2026-07-02)

## Background: two kinds of data with opposite needs

A trading platform produces two very different kinds of data, and the mistake this ADR prevents is treating them the same.

**Trading state** is data with accounting or state-machine meaning: orders, fills, positions, the lot ledger, reconciliation checkpoints. Its requirements are exactness (a fill of 0.1 BTC is 0.1, not 0.10000000000000000001), transactionality (a fill and its ledger posting must become visible together or not at all), and constraints (an order cannot be filled beyond its quantity, and the database should refuse to store a state that says otherwise).

**Time-series observations** are measurements streaming in over time: balance snapshots every minute, ticker prices, later per-fill marks for quant analysis. Their requirements are ingestion rate and cheap time-windowed queries ("average balance per hour over 90 days"). Losing or duplicating one observation is a smudge on a chart, not a broken ledger.

One database serves both badly. A row-oriented transactional store like Postgres can absolutely store time-series, but at high ingestion rates and long retention the table and index bloat make both jobs worse. A time-series engine like QuestDB ingests millions of rows per second, but its ingestion protocol is fire-and-forget (at-least-once, so duplicates happen), its numeric type on that path is float64, and it has no transactions spanning tables. Those properties are disqualifying for accounting and completely fine for observations.

| Requirement | Postgres | QuestDB (ILP path) |
|---|---|---|
| exact decimal arithmetic | `numeric` type, exact | float64, approximate |
| multi-table transactions | yes | no |
| constraints (CHECK, UNIQUE, FK) | yes | no |
| delivery of writes | transactional | at-least-once (duplicates possible) |
| sustained high-rate append + time-window queries | possible but degrades | designed for exactly this |

## Decision

Two databases, with a hard rule about which data goes where.

**Postgres** is the sole source of truth for anything with accounting or state-machine semantics. Access is through pgx/v5 with sqlc-generated queries; schema changes are goose migrations embedded in the binary via `embed.FS` and applied automatically at startup, so a deployed binary and its schema cannot drift apart. All money columns are `numeric`, mapped to `shopspring/decimal` in Go (ADR-0002).

**QuestDB** receives append-only time-series over ILP (InfluxDB Line Protocol) via HTTP, using go-questdb-client/v4. ILP is a text protocol built for ingestion speed; one balance observation on the wire looks like this:

```
balances,venue=coinbase,account=spot,currency=BTC total=0.5123,free=0.5,locked=0.0123 1720800000000000000
 |table | |------- symbols (indexed tags) ------| |-------- float64 fields --------| |timestamp, ns|
```

The client batches rows like this and flushes them in groups; the table and its columns are auto-created on first write, which is why QuestDB needs no migrations. The decimal-to-float64 conversion happens only at this edge, and it is acceptable only because nothing accounting-critical is ever read back from QuestDB. A chart can be off by a float rounding error; a ledger cannot.

The rule in one sentence: if getting a number wrong would corrupt money math or order state, it lives in Postgres; if it decorates a dashboard or feeds statistics, it lives in QuestDB.

To feel why the rule is not pedantry, walk one violation: suppose fills were stored only in QuestDB. ILP is at-least-once, so a network retry writes some fills twice; float64 rounds the quantities; there are no constraints to catch either. Now the ledger built from those fills carries phantom inventory at slightly wrong prices, every profit number inherits the error, and nothing anywhere can detect it because the corrupted store is also the reference. The same fills in Postgres are exact `numeric` values, written once (transactionally, deduplicated by constraint), and the QuestDB copy becomes what it should be: a disposable projection for charts, rebuildable from truth at any time.

### The checkpoint pattern that connects them

M1 keeps Postgres nearly empty on purpose: one table, `snapshot_checkpoints`, recording that a balance snapshot durably reached QuestDB. This solves a real problem that at-least-once ingestion creates: how do you know whether data actually arrived? The write ordering is what makes the checkpoint meaningful:

1. Write the snapshot rows to QuestDB and call `Flush`, which blocks until the server has accepted them.
2. Only then write the Postgres checkpoint row.

A checkpoint therefore asserts "this snapshot is in QuestDB". If the process dies between the two steps, the worst case is data in QuestDB with no checkpoint, which shows up as a gap to reconcile, never as a checkpoint pointing at missing data. The general shape (durable record in the truth store, written only after the lossy store confirmed) reappears in M2 as the transactional outbox (ADR-0008).

The M1 table also served a second purpose: it forced the whole goose + sqlc + testcontainers pipeline to exist and be proven before M2's orders, fills, and ledger arrived with real stakes.

## Consequences

- Two stores to run. `docker compose` provides both locally; a serious deployment carries both. Accepted as the cost of using each engine for what it is good at.
- Future quant analytics (z-scores, sigma events, drawdown, net-flow; see ROADMAP) read QuestDB. Nothing accounting-critical ever does. The boundary is directional: Postgres facts may be projected into QuestDB for charting, never the reverse.
- Grafana dashboards read QuestDB over PGWire (QuestDB speaks the Postgres wire protocol for queries) and read Prometheus for runtime metrics. Three query sources, one pane of glass.
