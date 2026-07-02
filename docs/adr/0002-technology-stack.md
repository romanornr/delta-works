# 0002 — Technology stack

**Status:** accepted (2026-07-02)

## Context

Long-lived multi-exchange trading platform (see [ROADMAP.md](../ROADMAP.md)). Priorities: correctness of money math, testability, resilience against flaky venues, and clean seams so order management, grid bots, and execution algos can be added without rework.

## Decision

| Concern | Choice | Rationale |
|---|---|---|
| Exchange connectivity | gocryptotrader behind ports (ADR-0003) | broad venue coverage without being load-bearing |
| Money math | shopspring/decimal; float64 only at the QuestDB analytics edge | exact accounting; fixed-point int64 hot path deferred |
| Config | koanf v2 (YAML + `DELTA__` env; secrets env-only) | modular, no global state |
| Logging | zerolog, aliased through `internal/log` | fast structured logging; single construction point |
| DI / lifecycle | uber fx | explicit modules, ordered start/stop hooks |
| Durable state | Postgres: pgx/v5 + sqlc + goose (ADR-0004) | type-safe SQL, embedded migrations |
| Time-series | QuestDB via ILP over HTTP (ADR-0004) | high-rate ingestion for analytics |
| Metrics | Prometheus client_golang + Grafana | standard pull-based observability |
| Resilience | x/time/rate → gobreaker/v2 → cenkalti/backoff/v5 (layering in ADR-0003) | venue protection independent of GCT internals |
| Messaging | in-process bus, NATS-shaped (ADR-0005) | defer infra until multi-process need |
| HTTP | stdlib net/http, Go 1.22+ ServeMux | 3 endpoints in M1; chi addable later, handlers stay stdlib-compatible |
| WebSocket | coder/websocket (when native adapters/streaming arrive) | maintained successor to nhooyr; preferred over gorilla for new code |
| Concurrency | context + errgroup; one actor goroutine per bot later | cancellation-first design |
| Testing | testcontainers-go for Postgres/QuestDB integration; `-race -shuffle=on` always | real dependencies in CI |

Enforcement: golangci-lint v2 (staticcheck, revive, gosec, misspell, depguard, …), govulncheck, `go mod tidy` drift check — all wired through `make ci` and GitHub Actions.

## Consequences

- One linter harness; depguard mechanically enforces architectural boundaries.
- Tools (sqlc, goose, govulncheck) pinned via go.mod `tool` directives; golangci-lint pinned as a binary.
- Newer alternatives consciously rejected for now: log/slog (zerolog faster), OpenTelemetry (Prometheus suffices), chi (stdlib suffices).
