# Delta Works Agent Guide

Purpose: short routing guide for humans and coding agents working in this repo.

## What this repo owns
- Delta Works v3 portfolio observation and transfer tracking logic
- domain model for holdings, transfers, and market data
- persistence port layer in `internal/storage/`
- QuestDB adapter in `internal/adapters/questdb/`
- GoCryptoTrader adapter in `internal/adapters/gct/`
- repo-local probes and learning commands in `cmd/`

## What this repo does not own
- production dashboards and external observability systems
- exchange implementations inside upstream GoCryptoTrader
- generic notes from old Cline/Roo workflows

## Read first
- `README.md`
- `docs/current-state.md`
- `docs/architecture.md`
- `docs/code-conventions.md`
- `config.example.yaml`

## Where to look first
- config model and loading: `internal/config/`
- canonical architecture boundaries: `internal/storage/`, `internal/adapters/`, `internal/domain/`
- QuestDB storage path: `internal/adapters/questdb/`
- exchange adapter path: `internal/adapters/gct/`
- example runtime entrypoint: `main.go`
- integration probes: `cmd/devprobe-*`
- repo-local learning commands: `cmd/learn/`

## Default commands
```bash
go test ./...
go vet ./...
go build ./...
go run ./cmd/devprobe-questdb
go run ./cmd/devprobe-storage
go run ./cmd/devprobe-gct
```

## Source-of-truth rules
- Canonical repo guidance lives in `README.md`, `docs/`, and this file.
- `private_docs/` is planning material, not authoritative runtime truth.
- Probe and learn commands are intentionally kept in-repo, but they are non-production helpers.
- Generated local artifacts must stay out of git, especially under `cmd/**/tls/` and local config files.

## Dangerous assumptions to avoid
- Do not describe this repo as an OEMS unless the code and docs are explicitly changed back in that direction.
- Do not bypass `internal/storage/` when describing persistence boundaries.
- Do not use `float64` for money or `time.Now()` directly in I/O-heavy logic.
- Do not treat `cmd/devprobe-*` or `cmd/learn/*` as production entrypoints.
