# delta-works

Multi-exchange crypto trading platform in Go. Currently: portfolio snapshot daemon (balances → QuestDB time-series, checkpoints → Postgres) with exchange connectivity via gocryptotrader quarantined behind ports. Next: order management, grid bots with exact PNL attribution, execution algos — see [docs/ROADMAP.md](docs/ROADMAP.md).

## Quickstart

```sh
cp config.example.yaml config.yaml           # adjust; API keys go in env, not the file
export DELTA__VENUES__BYBIT__API_KEY=...     # secrets are env-only
export DELTA__VENUES__BYBIT__API_SECRET=...

# native Postgres (5432) + QuestDB (9000), matching config.example.yaml:
make migrate-up && make run                  # daemon; metrics/health on :8080

# or the docker stack on offset ports (5433/9010):
make compose-up
DELTA__POSTGRES__DSN='postgres://oms:oms@localhost:5433/oms?sslmode=disable' make migrate-up
make run-docker
```

- QuestDB console: http://localhost:9010 · Grafana (optional): `docker compose -f deploy/docker-compose.yml --profile observability up -d` → http://localhost:3002
- Host ports are offset to coexist with natively installed Postgres/QuestDB/Grafana; see `deploy/docker-compose.yml`.

## Development

```sh
make ci                # full local gate: fmt-check, lint, vuln, test-race, tidy-check
make test-integration  # testcontainers (needs Docker)
```

Design docs: [docs/adr/](docs/adr/) (decisions) · [docs/specs/](docs/specs/) (milestone specs) · [AGENTS.md](AGENTS.md) (rules & tooling for AI assistants).
