# ADR-0006: Venue credentials come from secret files or environment, never the config file

## Status

Accepted (2026-07-03)

## Context

Venue API credentials were env-only. That worked for single-line keys but
has two problems. Operating the daemon locally required exporting variables
before every run, which invites shell-history leaks and wrapper scripts.
And some venues issue multiline credentials: Coinbase CDP keys are PEM
private keys, which do not fit environment variables or `KEY=value` env
files without fragile escaping.

Putting secrets in `config.yaml` was considered and rejected: config and
secrets have different lifecycles. Config should be freely shareable for
debugging, diffing, and backup; anything holding a secret is not.

## Decision

Each venue credential is provided in exactly one of two ways:

- a direct value (`api_key`, `api_secret`), intended for environment
  injection (`DELTA__VENUES__<V>__API_KEY`), or
- a path to a secret file (`api_key_file`, `api_secret_file`) holding one
  secret, possibly multiline, read at config load.

Setting both forms for one credential is a validation error, not a
precedence rule. Files are resolved before validation, so the rest of the
application only ever sees resolved values. Locally, secret files live in
the gitignored `secrets/` directory with mode 600.

## Consequences

- Multiline credentials (PEM keys) work unchanged.
- The same config works in deployment: container secret mounts and systemd
  credentials both deliver secrets as files.
- `make run` needs no exported variables.
- A leaked `config.yaml` reveals file paths, not secrets.
