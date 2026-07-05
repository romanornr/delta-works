# Architecture Decision Records

Every significant design choice gets a numbered, immutable ADR. To change a decision, write a new ADR that supersedes the old one — never rewrite history. AI assistants: read these before proposing architectural changes, and add an ADR in the same change when you make one.

Template: `NNNN-title.md` with sections **Status** (accepted/superseded-by-NNNN), **Context**, **Decision**, **Consequences**.

## Index

- [0001 — Clean restart: wipe main, original architecture](0001-clean-restart.md)
- [0002 — Technology stack](0002-technology-stack.md)
- [0003 — gocryptotrader quarantined behind ports](0003-gct-quarantine.md)
- [0004 — Postgres is truth, QuestDB is analytics](0004-postgres-truth-questdb-analytics.md)
- [0005 — In-process event bus now, NATS/JetStream later](0005-in-process-bus-nats-later.md)
- [0006 — Venue credentials come from secret files or environment, never the config file](0006-secret-files.md)
- [0007 — ConnectRPC control plane](0007-connectrpc-control-plane.md)
- [0008 — Transactional outbox for order events; state row plus transition log](0008-transactional-outbox.md)
