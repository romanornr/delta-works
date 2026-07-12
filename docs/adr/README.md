# Architecture Decision Records

An ADR is a short document that captures one significant design decision: the situation that forced a choice, the options that were on the table, the one that won, and what it costs. The point is that code shows *what* was built but cannot show *why*, and the why is what a future maintainer (or an AI assistant) needs before changing anything structural.

Rules for this directory:

- ADRs are immutable once accepted. To change a decision, write a new ADR that supersedes the old one and mark the old one `superseded-by-NNNN`. History is never rewritten, so the reasoning trail stays honest.
- Every significant design choice gets its ADR in the same change that implements it, not afterwards.
- AI assistants: read these before proposing architectural changes, and write one when you make one.

File naming is `NNNN-title.md`. Each ADR opens with a **Status** line and then explains the problem before the decision: a reader who knows Go but has never seen this project should be able to follow every one.

## Index

| ADR | Decision | One-line summary |
|---|---|---|
| [0001](0001-clean-restart.md) | Clean restart | `main` was wiped to an orphan commit; two legacy generations remain as read-only references, never templates |
| [0002](0002-technology-stack.md) | Technology stack | the full toolbox and the reasoning: decimal not float for money, sqlc not an ORM, fx for ordered lifecycle, testcontainers for real-database tests |
| [0003](0003-gct-quarantine.md) | gocryptotrader quarantine | the exchange framework is confined to one adapter package behind interfaces, enforced by the linter; includes the resilience layering (retry, breaker, rate limit) |
| [0004](0004-postgres-truth-questdb-analytics.md) | Postgres is truth, QuestDB is analytics | accounting state and time-series observations have opposite requirements, so they get different databases and a hard rule about which data goes where |
| [0005](0005-in-process-bus-nats-later.md) | In-process bus now, NATS later | a NATS-shaped pub/sub inside the process; at-most-once by contract, so durable data goes to Postgres and events are hints |
| [0006](0006-secret-files.md) | Secret files | venue credentials come from env vars or single-secret files, never `config.yaml`; multiline PEM keys and deployment secret mounts both just work |
| [0007](0007-connectrpc-control-plane.md) | ConnectRPC control plane | one protobuf contract serves gRPC, gRPC-Web and curl-able JSON on the same endpoint; every client goes through it, nothing bypasses it |
| [0008](0008-transactional-outbox.md) | Transactional outbox | order events are written to the database in the same transaction as the state change, and a relay delivers them to the bus; kills the dual-write lost-event problem |
