# delta-works

Multi-exchange crypto trading platform in Go. Currently: portfolio snapshot daemon (balances → QuestDB time-series, checkpoints → Postgres) with exchange connectivity via gocryptotrader quarantined behind ports. Next: order management, grid bots with exact PNL attribution, execution algos — see [docs/ROADMAP.md](docs/ROADMAP.md).

## Quickstart

```sh
cp config.example.yaml config.yaml           # adjust; API keys go in env, not the file
export DELTA__VENUES__BYBIT__API_KEY=...     # secrets are env-only
export DELTA__VENUES__BYBIT__API_SECRET=...
make compose-up                              # Postgres :5432, QuestDB :9000/:8812
make run                                     # daemon; metrics/health on :8080
```

- QuestDB console: http://localhost:9000 · Grafana (optional): `docker compose -f deploy/docker-compose.yml --profile observability up -d` → http://localhost:3000

## Development

```sh
make ci                # full local gate: fmt-check, lint, vuln, test-race, tidy-check
make test-integration  # testcontainers (needs Docker)
```

Design docs: [docs/adr/](docs/adr/) (decisions) · [docs/specs/](docs/specs/) (milestone specs) · [AGENTS.md](AGENTS.md) (rules & tooling for AI assistants).
