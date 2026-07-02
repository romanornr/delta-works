# 0004 — Postgres is truth, QuestDB is analytics

**Status:** accepted (2026-07-02)

## Context

The platform needs both durable trading state (orders, fills, positions, ledger, bot state, reconciliation checkpoints — must be exact and transactional) and high-rate time-series (balance snapshots, tickers, later fills/marks for quant analytics). One store cannot serve both well: QuestDB's ILP ingestion is at-least-once and float64-based; Postgres is not a time-series analytics engine.

## Decision

- **Postgres** (pgx/v5, sqlc-generated queries, goose migrations embedded via `embed.FS`, auto-run at startup) is the sole source of truth for anything with accounting or state-machine semantics. All money columns are `numeric`, mapped to shopspring/decimal.
- **QuestDB** (ILP over HTTP, go-questdb-client/v4) receives append-only time-series for dashboards and analytics. decimal→float64 conversion is acceptable **only** at this edge because QuestDB data is never accounting truth.
- M1 deliberately keeps Postgres near-empty: one `snapshot_checkpoints` table recording that a snapshot durably reached QuestDB — it enables gap detection/reconciliation and proves the goose+sqlc+testcontainers pipeline before M2's orders/fills/ledger arrive.
- Write ordering: QuestDB write + flush first, then the Postgres checkpoint — a checkpoint asserts "the data is in QuestDB".

## Consequences

- Two stores to run (docker compose provides both); accepted for a serious deployment.
- Future quant analytics (z-scores, sigma events, drawdown, net-flow — see ROADMAP) read QuestDB; nothing accounting-critical ever does.
- Grafana reads QuestDB over PGWire and Prometheus for runtime metrics.
