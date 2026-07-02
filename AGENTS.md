# AGENTS.md — delta-works

Multi-exchange trading platform in Go. Read `docs/ROADMAP.md` for where this is going, `docs/adr/` for why things are the way they are, and `docs/specs/` for the current milestone before changing anything significant.

## Commands

Everything goes through `make`:

| Target | Purpose |
|---|---|
| `make build` / `make run` | build/run `bin/deltad` |
| `make fmt` / `make lint` | gofumpt+goimports via golangci-lint v2 / full lint |
| `make test` / `make test-race` | unit tests (always `-shuffle=on`) |
| `make test-integration` | testcontainers tests (`-tags integration`, needs Docker) |
| `make generate` | sqlc codegen |
| `make migrate-up/-down/-status` | goose against `$DELTA__POSTGRES__DSN` |
| `make compose-up` | Postgres + QuestDB (`--profile observability` adds Prometheus/Grafana) |
| `make ci` | the full local gate: fmt-check, lint, vuln, test-race, tidy-check |

## Architecture rules (lint-enforced where possible)

- **Domain packages are pure**: `internal/domain/*` imports stdlib + shopspring/decimal only.
- **gocryptotrader only inside `internal/adapters/gct/`** — depguard-enforced (ADR-0003). Application code depends on `internal/ports` interfaces.
- **Money is `shopspring/decimal`, never float64** — the sole exception is the QuestDB ILP edge (analytics, not accounting truth; ADR-0004).
- **zerolog is imported only in `internal/log` and `cmd/`** — depguard-enforced; everything else uses the injected `log.Logger` alias.
- `context.Context` is the first parameter of anything that blocks.
- Table-driven tests; integration tests behind `//go:build integration`; live-venue tests behind `//go:build live` (manual only, never CI).
- Significant design choices get an ADR in `docs/adr/` in the same change.
- **Comments**: only where the code cannot say it itself, written in plain sentences an outside developer will still understand in five years. No em-dashes, no filler ("simply", "note that", "deliberately"), no narration of what the next line does.
- **The project may be renamed** — never scatter the brand into code. Identity strings live in exactly these places: `config.EnvPrefix` (`DELTA__`), `BINARY` in the Makefile (+ `cmd/deltad/` dir), `name:` in `deploy/docker-compose.yml`, and the module path (mechanical `go mod edit -module` + import rewrite). Metric names, bus subjects, and database/table names stay brand-neutral (`snapshot_*`, `bus_*`, `balances`, `tickers`).

## Tooling available to AI assistants

- **GitNexus** (`mcp__gitnexus__*`): the gocryptotrader source is checked out at `~/github/gocryptotrader` and indexed. Use it to understand GCT internals (engine boot, exchange interfaces, type shapes) when working on `internal/adapters/gct` — do not guess GCT APIs from memory.
- **Context7** (`mcp__context7__*`): fetch current docs for libraries (koanf, pgx, sqlc, fx, gobreaker, backoff, questdb client, testcontainers) instead of relying on training data.
- Legacy history: branches `backup`/`v1` + tag `legacy-final`; abandoned rewrite: branch `v3`. Reference only — never a template (ADR-0001).
