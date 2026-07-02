# 0003 — gocryptotrader quarantined behind ports

**Status:** accepted (2026-07-02)

## Context

gocryptotrader (GCT) provides connectivity to many venues, but it is a large framework with its own engine, config, database, and opinions. Depending on it directly throughout the codebase would make it load-bearing: its types, error semantics, and breaking changes would ripple everywhere (this happened in the legacy code). We must be able to replace it per-venue with native adapters later.

## Decision

- All application code depends only on the port interfaces in `internal/ports` (e.g. `Exchange`, `MarketDataReader`, `AccountReader`, later `OrderPlacer`/`PrivateStreamer`) using pure domain types.
- GCT is imported **only** inside `internal/adapters/gct/` — enforced mechanically by a depguard rule in `.golangci.yml`, not by convention.
- The adapter boots a trimmed GCT engine (exchange manager only: no GCT web UI, database, or comms) under fx lifecycle. `convert.go` is the single place GCT types meet domain types and is tested heavily.
- The adapter translates GCT failures into typed domain errors (`ErrVenueUnavailable`, `ErrAuth`, `ErrRateLimited`) so resilience layers can classify without knowing GCT.
- Resilience layering (outside the adapter): service-level retry (backoff/v5) → circuit breaker per venue (gobreaker/v2) → rate limiter (x/time/rate) → adapter. GCT's internal rate limiter is never trusted as the only guard.

The GCT source is checked out at `~/github/gocryptotrader` and indexed with GitNexus for adapter work (see AGENTS.md).

## Consequences

- A venue can be migrated to a native adapter by implementing the same ports; no caller changes.
- Slight duplication (our domain types mirror some GCT concepts) — accepted cost of independence.
- An accidental GCT import outside the adapter fails `make lint`.
