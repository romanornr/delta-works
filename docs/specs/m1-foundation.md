# Spec: M1 — Foundation + read-only exchange

**Status:** delivered 2026-07-02, except live-key verification (running with real venue credentials and confirming balances flow into QuestDB), which needs API keys in env.

## Goal

A daemon (`deltad`) that periodically snapshots balances from configured venues (via gocryptotrader behind ports), writes time-series to QuestDB, records durable checkpoints in Postgres, exposes Prometheus metrics and health endpoints, and shuts down gracefully — with the full engineering harness (lint, CI, integration tests, migrations, compose) in place from the first commit.

## Package layout

```
cmd/deltad/                 # main: fx app
internal/app/               # fx composition root
internal/config/            # koanf v2: config.yaml + DELTA__ env, typed Config, Validate()
internal/log/               # zerolog construction; exports Logger alias (sole zerolog import point)
internal/clock/ (+clocktest)# Clock port + deterministic fake
internal/telemetry/         # prometheus registry; HTTP: /metrics /healthz /readyz
internal/bus/               # in-proc NATS-shaped event bus (ADR-0005)
internal/domain/money/      # Currency, Amount (currency-mismatch-guarded decimal)   [pure]
internal/domain/instrument/ # VenueID, Instrument{Base,Quote,VenueSymbol,Rules}      [pure]
internal/domain/account/    # AccountType, AccountRef, Balance, Snapshot             [pure]
internal/domain/marketdata/ # Ticker                                                 [pure]
internal/ports/             # hexagon boundary: exchange + trading (M2 seam) + store ports
internal/adapters/gct/      # GCT engine lifecycle + port impls; convert.go sole contact point
internal/adapters/postgres/ # pgxpool, sqlc code, CheckpointStore, migrations/ (goose)
internal/adapters/questdb/  # ILP LineSender wrapper implementing SeriesWriter
internal/exchange/          # Registry(VenueID→Exchange) + WithRateLimit/WithBreaker decorators
internal/service/snapshot/  # snapshot poller (errgroup, one goroutine per venue+account)
```

Future homes (M2/M3, not created yet): `internal/domain/order`, `internal/domain/ledger`, `internal/oms`, `internal/strategy/grid`, `internal/execution`, `internal/service/reconcile`, `internal/httpapi`.

## Key behaviors

- **Domain purity:** domain packages import stdlib + shopspring/decimal only.
- **Ports split:** read ports (`MarketDataReader`, `AccountReader`) separate from the trading port (`OrderPlacer`, `PrivateStreamer` — compiled now, no implementor, locks the M2 seam). ClientOrderID is ours and is the idempotency key.
- **Resilience layering:** service retry (backoff/v5, elapsed cap < poll interval, auth errors permanent) → per-venue breaker (gobreaker/v2) → rate limiter (`rate.Limiter.Wait`) → GCT adapter.
- **Snapshot tick:** fetch balances → QuestDB write + flush → Postgres checkpoint (`ok|partial|failed`) → publish `snapshot.taken`. Checkpoint written only after the QuestDB flush.
- **Failure policy:** venue failures log/count and rely on breaker + next tick; infrastructure failures escalate to `fx.Shutdowner` (fail fast; compose/systemd restarts).
- **Metrics:** `snapshot_duration_seconds{venue}`, `snapshot_errors_total{venue}`, `snapshot_last_success_timestamp_seconds{venue}` (staleness-alertable), `bus_dropped_total`.

## Storage

- Postgres: `snapshot_checkpoints(id uuid PK, venue, account_type, taken_at, balance_count, status, error, created_at)` — goose migration embedded, auto-run at startup; sqlc + pgx/v5, numeric↔decimal.
- QuestDB (auto-created by ILP): `balances`(sym: venue, account, currency; dbl: total, free, locked; ts=taken_at), `tickers`(sym: venue, symbol; dbl: bid, ask, last, bid_size, ask_size).

## Verification

1. `make ci` green locally and in GitHub Actions (fmt-check, lint, vuln, test-race, tidy-check).
2. `make test-integration` green (testcontainers: Postgres migrations + checkpoint round-trip; QuestDB ILP round-trip).
3. `make compose-up && make run` with bybit keys in env → `balances` rows grow in QuestDB, ok checkpoints in Postgres, `/readyz` 200, clean SIGTERM exit.
