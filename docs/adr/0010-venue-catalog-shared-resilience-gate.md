# 0010: One venue catalog preserves capabilities and shares resilience

**Status:** accepted (2026-07-17)

## Context

The first runtime venue graph used a compound `ports.Exchange` interface that required account and market-data methods. The composition root wrapped that interface with separate rate-limiter and circuit-breaker types, registered it by venue ID, then built another slice for trading. The wrappers always implemented `OrderPlacer`, even when the wrapped adapter could not trade, so capability absence appeared only as `ErrTradingUnsupported` at call time.

This produced three representations of one venue. Snapshot derived targets from configuration and then looked their venue up in the registry. Order placement and reconciliation received service-specific venue values converted from the trading slice. Duplicate canonical IDs were silently ignored, and map iteration made construction order nondeterministic.

The broad gocryptotrader adapter also satisfies application interfaces structurally even when a particular GCT venue does not meet the full behavioral contract. For example, the order port requires client-order-ID idempotency, venue-wide unfiltered open-order lookup, and authoritative point lookup together. The existence of broadly shaped GCT methods does not prove those guarantees for every venue.

## Decision

`internal/venue` owns one deterministic catalog entry for every enabled canonical venue ID. A config-only preflight canonicalizes all enabled IDs and rejects duplicates before any adapter or network-capable resource is constructed. Catalog iteration is sorted by canonical ID.

An entry keeps its configured accounts and capability fields private. Accessors return `(capability, bool)` for account reading, market data, order placement, and private events. No exported nullable interface represents absence. An enabled venue with configured accounts requires an account reader. Market data remains an optional capability, and its domain, port, adapter conversion, and ticker-series writer remain in the system. Trading configuration requires an audited order capability. Missing required capabilities fail during the config-only preflight, before adapter construction. Private events remain optional even for a trading venue because periodic reconciliation is the correctness path.

The compound `ports.Exchange` interface, the old registry, the broad resilience wrappers, and the composition root's `exchangeProducts` and `tradingVenue` values are removed without compatibility shims. Snapshot receives direct `{venue, account, reader}` targets. The existing order and reconciliation services consume a catalog-owned capability view; they do not introduce temporary service-specific venue conversions.

Configured account strings retain their existing behavior. Translation preserves order and duplicates, performs no new startup validation or deduplication, and leaves unsupported-account classification at the adapter call boundary. This structural change does not turn an adapter-time account failure into a startup failure.

Each entry owns one request gate for all of its synchronous capabilities. Thin account, market-data, and order wrappers implement only the capability they guard and share the same limiter and circuit breaker. The call order remains:

```text
service retry
  -> shared venue breaker
    -> shared venue limiter
      -> adapter
```

The breaker therefore rejects an open circuit before waiting for or consuming a limiter token. A local limiter wait failure is a breaker success because no venue request occurred. Breaker construction keeps the canonical venue ID as its name, a 30-second timeout, the existing success classifier, and zero values for every other gobreaker setting. The success classifier continues to treat `ErrAuth`, `ErrUnsupportedAccount`, `ErrNotFound`, `ErrNoVenueOrderID`, caller cancellation, and local limiter failure as non-venue failures. Service retry remains outside the gate.

Private stream setup does not pass through the request gate. A stream is a long-lived session with different setup, lifetime, and reconnect semantics from a finite request. Its availability does not consume synchronous rate budget or change the request breaker's state.

## Source-verified GCT capabilities

The GCT adapter boundary owns a conservative, per-venue support table. Structural interface satisfaction and feature flags alone are not evidence. A capability is exposed only after the selected GCT source is checked against the complete application port contract.

The re-audit used the indexed upstream checkout at commit `65951fd731e183c1ac668130eb2e89497d96529f` and the effective `go.mod` replacement, `github.com/romanornr/gocryptotrader` at commit `c224359c0964f849dcde4d1236017121bca0b5e9`. The selected revision removes no capability, but the re-audit corrects four previously overstated market-data booleans and one private-event boolean.

The prior audit overstated Coinbase order support because typed missing-order classification was absent at both the indexed revision and the prior effective replacement, `github.com/romanornr/gocryptotrader` at commit `fa94ed8d0137084315f909475c23f902f13d43f0`. Commit `c224359c0964f849dcde4d1236017121bca0b5e9` completes the order contract by mapping parsed `NOT_FOUND` responses on lookup and cancellation to the typed order-not-found sentinel while leaving `INVALID_ARGUMENT` unmapped. All other audited capability-contract behaviors agree between the indexed checkout and the selected revision; the selected fork also changes Coinbase signing-key support without changing a capability boolean.

At the selected revision, 20 of the 24 known engine venues support the complete market-data contract, all except Bitflyer support the account contract, only Coinbase is proven to support the complete order contract, and no private-event capability is currently proven complete. Bithumb, Gemini, Huobi, and LBank expose only partial ticker observations under the current domain contract because they omit last-trade or top-of-book fields; revisit those capability claims when market data gains a consumer. Redefining the ticker contract is deferred.

The Coinbase websocket conversion loses venue-provided fill price, fee, and fill ID and can substitute limit price for average price. That would post ledger lots at zero or the wrong cost basis after cumulative quantity advances, which reconciliation cannot repair. Orders therefore rely on the existing 30-second reconciliation loop, the proven-live correctness path. The conversion fix is deferred to the planned Coinbase stream investigation, not this amendment. Unknown venue IDs fail closed before adapter construction.

Every change to the selected GCT revision requires the table to be re-audited against both the indexed checkout and the code actually selected by `go.mod`. Adding a table row or capability requires source evidence for the full port contract. A newly added GCT method or a successful Go type assertion is not enough.

## Relationship to prior decisions

This decision partially supersedes ADR-0003 only for its compound `ports.Exchange` shape and capability-erasing resilience wrappers. ADR-0003 remains accepted for GCT quarantine, application-owned ports, adapter dependency direction, typed boundary errors, and service retry outside breaker outside limiter. GCT imports remain confined to `internal/adapters/gct`.

This slice replaces the old runtime path instead of retaining parallel catalog and registry paths.

## Consequences

- One catalog entry is the runtime identity and capability graph for a venue.
- Capability absence is visible before services start and cannot leak through an embedded raw adapter.
- Account, market-data, and order requests intentionally influence one venue-wide breaker and consume one venue-wide limiter budget.
- A native adapter can supply any valid subset of capabilities without pretending to implement the rest.
- Snapshot no longer reconstructs targets or performs a redundant registry lookup.
- Order and reconciliation keep their current behavior while consuming the same catalog-owned venue view.
- Conservative GCT capability claims may reject a venue that has broad methods but lacks proof for the application's complete contract.
- Market data stays available for the future producer; this decision does not claim ticker ingestion is active.
