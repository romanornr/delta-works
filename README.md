# Delta Works

Delta Works v3 is a Go application for portfolio observation and transfer tracking across cryptocurrency exchanges.
It captures holdings over time, syncs transfer history, and stores data in QuestDB for analysis and Grafana-style visualization.

This repo is not currently an OEMS / trading-strategy system. The canonical scope is:
- periodic portfolio snapshots
- exchange transfer synchronization
- clean adapter boundaries for QuestDB and GoCryptoTrader
- an app/runtime skeleton built with Uber Fx

## Current status

The project is in an adapter-first phase.
The QuestDB storage adapter and GoCryptoTrader exchange adapter are the active integration surfaces, with dev probes under `cmd/` used to validate assumptions before more app-layer orchestration is added.

See:
- `docs/current-state.md`
- `docs/architecture.md`
- `docs/code-conventions.md`
- `AGENTS.md`

## Quick start

```bash
cp config.example.yaml config.yaml
go test ./...
go run main.go
```

## Useful commands

```bash
go test ./...
go vet ./...
go build ./...
go run ./cmd/devprobe-questdb
go run ./cmd/devprobe-storage
go run ./cmd/devprobe-gct
```

## Repository layout

```text
internal/
  config/          configuration loading and validation
  observability/   zerolog wiring
  clock/           time abstraction
  clocktest/       deterministic test clock
  errs/            sentinel errors
  domain/          portfolio, market, transfer domain types
  storage/         persistence ports
  adapters/
    questdb/       QuestDB-backed storage adapter
    gct/           GoCryptoTrader-backed exchange adapter
cmd/
  devprobe-*/      integration and boundary probes
  learn/           repo-local learning / experimentation commands
```

## Configuration

- Example config: `config.example.yaml`
- Local config: `config.yaml` or `config.yml` (gitignored)
- Override path: `DELTAWORKS_CONFIG_PATH=/path/to/config.yaml`

## Planning notes

Private planning material remains under `private_docs/` and is intentionally not the canonical public repo guidance.
The canonical repo guidance now lives in `README.md`, `docs/`, and `AGENTS.md`.
