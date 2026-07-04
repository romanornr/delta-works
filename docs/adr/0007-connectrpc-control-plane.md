# ADR-0007: ConnectRPC control plane

## Status

Accepted (2026-07-04)

## Context

The platform needs a management surface: a CLI now, a TUI when grid bots
arrive (M3), a web UI later (M4). All of them need typed queries, commands
that move real money, and a live event feed. The daemon is long-running and
headless, so every client attaches over a wire protocol.

Options considered:

- **Plain HTTP/JSON with SSE or WebSocket.** No toolchain, but the contract
  lives in hand-synchronized types across Go and (later) TypeScript, drift
  surfaces at runtime, and streaming is bolted on. Wrong risk profile for
  order-touching commands.
- **NATS request-reply.** The bus interface already mirrors NATS (ADR-0005),
  but as a client transport it forces a broker deployment today, browsers
  cannot speak it, and payloads are schema-free bytes. NATS remains the
  planned internal event fabric; it is upstream of the API, not a
  replacement for it.
- **Pure gRPC.** Typed and streaming, but browsers need a translation proxy
  forever and endpoints are not curl-able.
- **ConnectRPC.** One protobuf contract; a single net/http handler speaks
  gRPC, gRPC-Web and the Connect protocol (plain POST+JSON, curl-able).
  Generated Go clients now, generated TypeScript against the same endpoint
  later, no proxy.

## Decision

- The control plane is ConnectRPC over protobuf. Protos live in `proto/`,
  generated code in `internal/api/gen/` (never hand-edited); buf drives
  codegen, lint and breaking-change detection, all pinned as go.mod tools.
- **No client bypasses the API.** CLI, TUI and web UI all speak the same
  contract; nothing gets a privileged in-process side-channel. If a client
  needs something the API lacks, the API grows.
- The wire package is `control.v1`: brand-neutral like metric names and bus
  subjects, so renaming the project never breaks wire compatibility.
- The server is disabled unless `api.addr` is set. `unix://` sockets are
  the default posture: file permissions (0600) are the auth model. TCP is
  for trusted networks only until token auth (connectrpc/authn-go) is added
  alongside a remote-access need.
- Request validation is declared in the schema (protovalidate) and enforced
  by an interceptor before handlers run.
- Event streaming is a server-streaming RPC fed by the internal bus and
  inherits its at-most-once contract: slow clients drop events, never stall
  the daemon. Anything that must not be lost is read back from Postgres.
- Money crosses the wire as decimal strings, never floats (ADR-0004).

## Consequences

- buf joins sqlc in `make generate`; `buf lint` and `buf breaking` (against
  main) run in `make ci`. Contract drift between daemon and clients becomes
  a build-time failure.
- New event types are added as `oneof` arms in the `Event` envelope; arms
  are never renumbered.
- When NATS replaces the in-proc bus (ADR-0005), the API server becomes a
  NATS subscriber; clients are unaffected.
- The M4 web UI generates TypeScript (connect-es) from the same protos and
  needs no gateway; cors-go is added only if it is served cross-origin.
