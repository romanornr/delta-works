# 0007: ConnectRPC control plane

**Status:** accepted (2026-07-04)

## Background: what a control plane is and why this daemon needs one

The daemon is long-running and headless: it holds the exchange connections, the database pools, and (from M2) the only path that places orders. Humans and tools need a way to talk to it: a CLI today, a TUI when grid bots arrive (M3), a web UI later (M4). The management surface those clients share is called the control plane, and it has three jobs that pull on the protocol choice:

1. Typed queries: "list orders", "show the latest snapshot", with real field types, not stringly JSON both sides hope matches.
2. Commands that move real money: place order, cancel order. These need schema-enforced validation and contracts that cannot silently drift between daemon and client.
3. A live event feed: fills and state changes streaming to a watching client.

The commands are the reason the bar is high. A drifted contract on a read endpoint shows the wrong number on a screen; a drifted contract on `PlaceOrder` trades the wrong amount.

## The options, and what each one actually costs

**Plain HTTP/JSON with SSE or WebSocket for streaming.** No toolchain, everyone knows it. The cost is that the contract lives in hand-written types maintained twice, in Go now and TypeScript later, and nothing checks they agree. Drift is discovered at runtime by whoever hits the mismatched field. Streaming arrives as a second mechanism (SSE or WebSocket) with its own reconnect and framing story. Acceptable risk for a blog API; wrong risk profile for order commands.

**NATS request-reply.** The internal bus is already NATS-shaped (ADR-0005), so this looks natural. But as a client transport it forces a broker deployment on day one, browsers cannot speak NATS, and payloads are schema-free bytes so the drift problem returns. NATS remains the planned internal event fabric, upstream of the API, not a replacement for it.

**Pure gRPC.** Typed contracts, first-class streaming, mature tooling. Two costs: browsers cannot speak native gRPC (it needs HTTP/2 trailers that browser APIs do not expose), so a web UI forever requires a translation proxy (grpc-web + Envoy or similar); and endpoints are not curl-able, which hurts exactly when debugging a live incident.

**ConnectRPC.** A newer implementation of the same idea from the team that maintains buf. One protobuf contract, and the generated server handler is a normal `net/http` handler that speaks three protocols on the same endpoint: native gRPC, gRPC-Web, and the Connect protocol, which is a plain HTTP POST with a JSON body. That last one means this works during an incident:

```
curl -X POST --unix-socket /run/delta/api.sock \
  http://localhost/control.v1.SnapshotService/GetLatestSnapshot \
  -H 'Content-Type: application/json' -d '{}'
```

Typed Go clients are generated now; typed TypeScript clients (connect-es) are generated later from the same protos and talk to the same endpoint with no proxy.

| | HTTP/JSON | NATS req-reply | gRPC | ConnectRPC |
|---|---|---|---|---|
| contract checked at build time | no | no | yes | yes |
| streaming built in | bolted on | yes | yes | yes |
| browser without a proxy | yes | no | no | yes |
| curl-able | yes | no | no | yes |
| extra infrastructure | none | broker | proxy for web | none |

## Decision

- The control plane is ConnectRPC over protobuf. Protos live in `proto/`, generated code in `internal/api/gen/` and is never hand-edited. buf drives code generation, lint, and breaking-change detection, pinned as go.mod tools; `buf breaking` compares against main in CI, so an incompatible proto change fails the build instead of a client.
- **No client bypasses the API.** CLI, TUI, and web UI speak the same contract; nothing gets a privileged in-process side channel. If a client needs something the API lacks, the API grows. One surface means one place to validate, authorize, and audit anything that touches money.
- The wire package is `control.v1`, brand-neutral like metric names and bus subjects, so renaming the project (a stated possibility, see AGENTS.md) never breaks wire compatibility.
- The server is disabled unless `api.addr` is configured. The default posture is a `unix://` socket: the socket file's permissions (0600) are the authentication model, which is exactly as strong as local user separation and involves zero token management. TCP is for trusted networks only until token auth (connectrpc/authn-go) is added alongside an actual remote-access need.
- Request validation is declared in the schema with protovalidate annotations and enforced by an interceptor before any handler runs, so handlers never see an invalid request and validation rules live next to the fields they constrain.
- Event streaming is a server-streaming RPC fed by the internal bus, and it inherits the bus's at-most-once contract (ADR-0005): a slow client drops events rather than stalling the daemon. Anything that must not be lost is read back from Postgres (ADR-0004).
- Money crosses the wire as decimal strings, never floats (ADR-0002, ADR-0004). Protobuf's `double` is IEEE 754 with the same defects as float64.

## Consequences

- buf joins sqlc in `make generate`; `buf lint` and `buf breaking` run in `make ci`. Contract drift between daemon and clients is a build failure, not a support ticket.
- New event types are added as `oneof` arms in the `Event` envelope. Arm numbers are append-only and never reused, because a recycled field number silently misparses old clients.
- When NATS replaces the in-process bus (ADR-0005), the API server becomes a NATS subscriber; clients notice nothing.
- The M4 web UI generates TypeScript from the same protos and connects directly; cors-go is added only if it is ever served cross-origin.
