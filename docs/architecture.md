# Architecture

## Scope

Delta Works v3 is a portfolio observation and transfer tracking system for cryptocurrency exchanges.
The current codebase centers on two active integration surfaces:
- QuestDB for storage
- GoCryptoTrader for exchange connectivity

## Layered structure

```text
main.go
  -> fx.New(...)
      -> config.Module
      -> observability.Module
      -> adapters/questdb (storage implementation)
      -> adapters/gct (exchange implementation)
      -> app (planned orchestration layer)
```

## Package boundaries

- `internal/domain/`
  - pure domain types and value logic
  - no imports from other internal layers
- `internal/errs/`
  - sentinel errors and error grouping
- `internal/clock/`
  - time abstraction for testability
- `internal/storage/`
  - persistence ports only
  - depends on domain types, not infrastructure implementations
- `internal/adapters/questdb/`
  - QuestDB-backed implementation of storage interfaces
- `internal/adapters/gct/`
  - exchange adapter around GoCryptoTrader
- `internal/app/`
  - future orchestration and use-case logic
- `internal/config/`
  - configuration loading, normalization, validation
- `internal/observability/`
  - logging setup

## Dependency direction

These rules are intentional and should stay stable:

```text
domain      -> depends on nothing internal
errs        -> depends on nothing internal
clock       -> depends on nothing internal
storage     -> depends on domain only
adapters    -> depend on storage/domain/config as needed
app         -> depends on storage/domain/clock/exchange-facing interfaces
main.go     -> wires modules via Fx
```

## Current patterns in use

### Storage interface composition

`internal/storage/` uses fine-grained interfaces plus a top-level facade:
- `SnapshotWriter`, `SnapshotReader`, `SnapshotStore`
- `TransferWriter`, `TransferReader`, `TransferStore`
- `Store` exposes `Snapshots()`, `Transfers()`, `Ping(ctx)`, `Close(ctx)`

This keeps services dependent on the smallest interface they need.

### Adapter-owned lifecycle

The QuestDB adapter owns connectivity and lifecycle at the top-level store, while snapshot and transfer logic stay behind accessor methods.

### Probe-first integration

External integrations are de-risked with small commands under `cmd/` before expanding the app layer.
Current probes:
- `cmd/devprobe-questdb`
- `cmd/devprobe-storage`
- `cmd/devprobe-gct`

### Repo-local learning commands

`cmd/learn/` holds experimental and educational commands that are intentionally kept in-repo.
They are not production runtime entrypoints.

## Current implementation flow

### Snapshot path (target shape)

```text
scheduler/service
  -> exchange adapter fetches balances
  -> balances convert into portfolio.Holding values
  -> snapshot built with capture time
  -> storage.Snapshots().Write(ctx, snapshot)
```

### Transfer path (target shape)

```text
scheduler/service
  -> storage.Transfers().LastTime(...)
  -> exchange adapter fetches transfers since cursor
  -> normalize and deduplicate
  -> storage.Transfers().WriteBatch(ctx, transfers)
```

### Holding terminology

The portfolio model uses `Holding` terminology, not `Position` terminology.
Prefer:
- `Holding`
- `Holdings`
- `AddHolding`
- `NonZeroHoldings`

## Runtime truth

For actual current behavior, prefer canonical code over old planning notes.
High-signal starting points:
- `internal/config/config.go`
- `internal/storage/store.go`
- `internal/domain/portfolio/types.go`
- `internal/adapters/questdb/`
- `internal/adapters/gct/`
- `cmd/devprobe-*`
