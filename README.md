# delta-works

A multi-exchange cryptocurrency trading platform in Go, built to run real money carefully.

## What this is

One long-running daemon (`deltad`) connects to cryptocurrency exchanges, and a CLI (`deltactl`) controls it over a typed API. The project is built milestone by milestone, each named for what the operator can do once it ships, and each specified before it is implemented:

| Milestone | What it delivers | Status |
|---|---|---|
| Account watch | the engineering foundation plus a read-only daemon: balance snapshots into a time-series database, durable checkpoints, metrics, health, graceful shutdown | delivered |
| Manual trading | order placement through the CLI: a persisted order state machine, private event streams, a reconciliation loop, a per-bot inventory ledger with exact cost basis | delivered |
| Grid bots | concurrent grid trading bots with exact profit attribution | next |
| Execution and cross-venue | execution algorithms, cross-venue strategies, a web UI | planned |

The full plan with reasoning is in [docs/ROADMAP.md](docs/ROADMAP.md).

## How it is put together

```mermaid
graph LR
    subgraph daemon
        SVC[services: snapshot, order, reconcile, outbox] --> PORTS[ports: interfaces]
        PORTS --> GCT[gct adapter]
        SVC --> PG[(Postgres: truth)]
        SVC --> QDB[(QuestDB: analytics)]
        API[ConnectRPC control plane] --> SVC
    end
    GCT --> EX[exchanges]
    CLI[deltactl] --> API
```

Four rules shape everything, each recorded as an architecture decision record with its full reasoning:

1. Money is never a float. All accounting math uses exact decimals; the reasoning, including the classic `0.1 + 0.2` demonstration, is in [ADR-0002](docs/adr/0002-technology-stack.md).
2. The exchange library (gocryptotrader) is confined to one adapter package behind interfaces, enforced by the linter, so it can be replaced per venue without touching callers ([ADR-0003](docs/adr/0003-gct-quarantine.md)).
3. Postgres holds accounting truth; QuestDB holds time-series for dashboards and analytics. Data that would corrupt money math if wrong never lives in the analytics store ([ADR-0004](docs/adr/0004-postgres-truth-questdb-analytics.md)).
4. Every client, including our own CLI, goes through the same typed API. Nothing gets a privileged side channel ([ADR-0007](docs/adr/0007-connectrpc-control-plane.md)).

The ADR index at [docs/adr/](docs/adr/) explains every significant decision; the milestone specs at [docs/specs/](docs/specs/) are the normative design documents.

## Quickstart

```sh
cp config.example.yaml config.yaml    # adjust venues and ports

# credentials: either environment variables...
export DELTA__VENUES__COINBASE__API_KEY=...
export DELTA__VENUES__COINBASE__API_SECRET=...
# ...or files (required for multiline PEM keys; see docs/adr/0006-secret-files.md):
#   venues.coinbase.api_key_file: secrets/coinbase.key

# native Postgres (5432) + QuestDB (9000), matching config.example.yaml:
make migrate-up && make run           # daemon; metrics and health on :8080

# or the docker stack on offset ports (5433/9010):
make compose-up
DELTA__POSTGRES__DSN='postgres://oms:oms@localhost:5433/oms?sslmode=disable' make migrate-up
make run-docker
```

- QuestDB console: http://localhost:9010. Grafana (optional): `docker compose -f deploy/docker-compose.yml --profile observability up -d`, then http://localhost:3002.
- Host ports are offset so the docker stack can coexist with natively installed Postgres, QuestDB, and Grafana; see `deploy/docker-compose.yml`.

## Development

```sh
make ci                # the full local gate: fmt-check, lint, proto-lint, vuln, test-race, tidy-check
make test-integration  # integration tests against real Postgres/QuestDB in containers (needs Docker)
make generate          # regenerate sqlc queries and protobuf code
```

Read [AGENTS.md](AGENTS.md) before contributing: it holds the build commands, the architecture rules the linter enforces, and the conventions (table-driven tests, comment style, the net-negative diff preference). Contributions by AI assistants follow the same file.
