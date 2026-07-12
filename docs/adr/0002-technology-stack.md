# 0002: Technology stack

**Status:** accepted (2026-07-02)

## Context

This is a long-lived multi-exchange trading platform (see [ROADMAP.md](../ROADMAP.md)). The stack was chosen against four priorities, in order: correctness of money math, testability, resilience against flaky venues, and clean seams so order management, grid bots, and execution algorithms can be added without rework. Every row in the table below is a decision; the sections after it explain the ones where the reasoning teaches something that is not obvious from the one-line rationale.

## Decision

| Concern | Choice | Rationale |
|---|---|---|
| Exchange connectivity | gocryptotrader behind ports (ADR-0003) | broad venue coverage without letting the framework leak everywhere |
| Money math | shopspring/decimal; float64 only at the QuestDB analytics edge | exact accounting; see below |
| Config | koanf v2 (YAML + `DELTA__` env; secrets via env or files, ADR-0006) | modular, no global state |
| Logging | zerolog, aliased through `internal/log` | fast structured logging; single construction point |
| DI / lifecycle | uber fx | explicit modules, ordered start/stop hooks; see below |
| Durable state | Postgres: pgx/v5 + sqlc + goose (ADR-0004) | type-safe SQL without an ORM; see below |
| Time-series | QuestDB via ILP over HTTP (ADR-0004) | high-rate ingestion for analytics |
| Metrics | Prometheus client_golang + Grafana | standard pull-based observability |
| Resilience | x/time/rate, gobreaker/v2, cenkalti/backoff/v5 (layering in ADR-0003) | venue protection independent of GCT internals |
| Messaging | in-process bus, NATS-shaped (ADR-0005) | defer infrastructure until a second process exists |
| HTTP | stdlib net/http, Go 1.22+ ServeMux | three endpoints in M1; handlers stay stdlib-compatible if chi is ever wanted |
| WebSocket | coder/websocket (when native adapters arrive) | maintained successor to nhooyr; preferred over gorilla for new code |
| Concurrency | context + errgroup; one actor goroutine per bot later | cancellation-first design |
| Testing | testcontainers-go for Postgres/QuestDB; `-race -shuffle=on` always | see below |
| Order IDs | oklog/ulid/v2 in `internal/id` | sortable, uniform idempotency keys; domain stays pure |

Enforcement: golangci-lint v2 (staticcheck, revive, gosec, misspell, depguard, and more), govulncheck, and a `go mod tidy` drift check, all wired through `make ci` and GitHub Actions.

## Why never float64 for money

Floating-point numbers cannot represent most decimal fractions exactly, because they are binary fractions underneath. The canonical demonstration, in any language with IEEE 754 doubles:

```
0.1 + 0.2 == 0.30000000000000004
```

For graphics or physics that error is irrelevant. For accounting it compounds: sum ten thousand trade amounts and the error is no longer in the sixteenth digit, and worse, two code paths that compute "the same" total can disagree, so balance checks fail intermittently and irreproducibly. Exchanges themselves send decimal strings in their APIs for exactly this reason.

`shopspring/decimal` stores numbers as an integer coefficient plus a base-ten exponent, so `0.1` is exactly `1 * 10^-1` and arithmetic stays exact. The cost is speed and allocations, which does not matter at this system's trade rates. If a future hot path needs faster math, the plan of record is fixed-point int64 (amounts stored as integer multiples of a smallest unit), not float.

The one sanctioned exception: the QuestDB write path converts to float64 at the last possible moment, because QuestDB's ILP protocol takes doubles and that database is analytics, not accounting truth (ADR-0004). A chart being off by a rounding error is acceptable; a ledger being off is not.

## Why sqlc instead of an ORM

An ORM generates SQL from Go code at runtime. sqlc does the reverse: you write real SQL in `.sql` files, and it generates type-checked Go functions from them at build time. The trade:

| | ORM | sqlc |
|---|---|---|
| You write | Go method chains | SQL |
| SQL you get | generated, discovered at runtime | exactly what you wrote |
| Type safety | partial, often via reflection | full, at compile time |
| When a query is wrong | at runtime, in production if untested | at code generation or compile time |
| Query tuning (indexes, EXPLAIN) | fight the generator | ordinary SQL work |

For a system whose correctness lives in its queries (row locking, `ON CONFLICT` idempotency, partial indexes, advisory locks in M2), hiding the SQL is the wrong direction. sqlc keeps the SQL visible and reviewable while removing the hand-written scanning boilerplate where mistakes hide. goose handles schema migrations as plain SQL files embedded in the binary, applied in order at startup.

## Why a dependency-injection framework at all

fx wires the application together from constructors: each component declares what it needs as parameters and fx builds the graph, then starts lifecycle hooks in dependency order and stops them in reverse. Two things justify the dependency over hand-wiring in `main()`:

1. Ordered shutdown. A trading daemon must stop accepting API calls before it stops the services behind them, and must keep the database pool alive until every service using it has returned. Reverse-order lifecycle hooks give that for free; hand-written shutdown ordering is where subtle bugs live.
2. Wiring tests. Because construction is data (a list of constructors), a test can build the whole application graph with a fake venue and assert properties like "with trading disabled, no private websocket component is even constructed" (this becomes real in M2).

## Why testcontainers and those test flags

Mocking a database validates your mock, not your SQL. testcontainers-go starts a real Postgres (and QuestDB) in Docker for integration tests, so `ON CONFLICT` behavior, constraint violations, and lock interactions are tested against the engine that runs in production. These tests build behind an `integration` tag and run in CI.

`-race` enables Go's race detector, which catches concurrent memory access bugs that pass every ordinary test until they corrupt something in production. `-shuffle=on` randomizes test order every run, so tests that secretly depend on each other's leftover state fail early instead of years later. Both are always on, not optional.

## Consequences

- One linter harness; depguard mechanically enforces the architectural boundaries other ADRs declare (a stray gocryptotrader import outside its adapter fails `make lint`, it does not wait for review).
- Tools (sqlc, goose, govulncheck) are pinned via go.mod `tool` directives; golangci-lint is pinned as a binary. Builds do not drift with the network.
- Alternatives consciously rejected for now, with the trigger that would reopen them: log/slog (revisit if zerolog stagnates), OpenTelemetry tracing (revisit when there is a second process to trace across), chi (revisit when the HTTP surface outgrows ServeMux).
