# 0009: Models have owners and ports are sized to consumers

**Status:** accepted (2026-07-17)

## Context

ADR-0003 established that application code depends on ports while gocryptotrader remains behind its adapter. It did not say where the models crossing those ports belong or how much of a concrete adapter each consumer should see.

The initial order store put persisted order records, order queries, and ledger outcomes in `internal/ports`, then exposed every order operation through one `OrderStore` interface. That made the ports package a universal DTO owner and made command, reconciliation, and API tests implement methods their consumers never called. The concrete Postgres store needs all of those operations, but its union of methods is not a useful application dependency.

## Decision

Models belong to the domain or application capability that gives them meaning. The order domain owns its persisted record, query, and event-application result. That result embeds the ledger-owned outcome of applying a fill to inventory. Ports refer to those owner models instead of redeclaring them as transport-neutral DTOs.

Each application consumer depends on the smallest adapter behavior it uses. Order commands, event application, reconciliation reads, and API queries therefore use separate interfaces. A concrete adapter may implement several consumer ports and be bound to each of them, but no consumer receives their union.

Pure domain models contain only domain concepts. Infrastructure-native identifiers and transport representations remain with the application capability, adapter, or wire owner that gives them meaning. Domain packages retain the import boundary enforced by the linter: standard library, sibling domain packages, and `shopspring/decimal` only.

Database-generated lot IDs are not an application result. The previous `OpenedLotID` existed only to ease adapter tests; those tests now follow the durable order-to-fill-to-lot relationship.

PR3 continues this ownership migration for snapshot checkpoints, series writes, outbox messages, and existing event payloads. That continuation must preserve the same dependency direction and must not move infrastructure identifiers into a pure domain package.

This is a structural change only. Protobuf fields, outbox JSON, database schemas, stored values, transaction boundaries, and order-state behavior remain unchanged.

## Consequences

- A model's package identifies the capability responsible for its meaning and invariants.
- Consumer fakes implement only the behavior under test.
- Postgres still supplies all order persistence, but compile-time assertions prove each narrow port separately.
- Adding an operation to one consumer no longer expands unrelated consumers and their tests.
- Some concrete constructors require multiple interface bindings and some services receive more than one store parameter.

## Relationship to prior decisions

This ADR refines ADR-0003's application-owned-port rule by defining model ownership and consumer sizing. It does not supersede the gocryptotrader quarantine, the adapter dependency direction, or any other part of ADR-0003.
