# 0006: Venue credentials come from secret files or environment, never the config file

**Status:** accepted (2026-07-03)

## Background: what makes secret handling hard

An exchange API key pair is a bearer credential: whoever holds it can trade with the account's money. That puts three requirements on how the daemon receives credentials, and they pull in different directions:

1. Secrets must not end up in places that get copied around: git history, shell history, log files, config backups, pastebins during debugging.
2. Operating the daemon must stay convenient, locally and deployed. A scheme that requires exporting six variables before every run gets bypassed with a wrapper script, and the wrapper script becomes the new leak.
3. Real credentials have inconvenient shapes. Coinbase CDP keys are PEM private keys: multiple lines, headers, base64 body. Environment variables and `KEY=value` env files handle multiline values only with fragile escaping that breaks silently.

The scheme this replaced was env-only, which met requirement 1 but failed 2 (export-before-every-run invites wrapper scripts and shell-history leaks) and failed 3 outright (PEM keys).

### Why not just put them in config.yaml

Considered and rejected for a lifecycle reason, not a technical one. A config file wants to be freely shareable: attached to a bug report, diffed against a teammate's, committed as an example, backed up carelessly. The moment one field of it is secret, every one of those actions becomes a potential leak, and the whole file inherits the handling rules of its most sensitive field. Keeping secrets physically outside the config file means the config file stays harmless.

### Why not a secrets manager

Vault or a cloud secret manager adds an external service dependency, an authentication bootstrap problem (the daemon needs a credential to fetch its credentials), and operational surface, for a single-operator single-host deployment. Files with mode 600 deliver the same isolation here. The design below does not block a manager later: a manager agent that materializes secrets as files (the common deployment pattern) plugs into the file path unchanged.

## Decision

Each venue credential is provided in exactly one of two ways:

| Form | Config keys | Intended for |
|---|---|---|
| direct value | `api_key`, `api_secret` | environment injection: `DELTA__VENUES__<VENUE>__API_KEY` |
| file path | `api_key_file`, `api_secret_file` | one secret per file, possibly multiline (PEM), read at config load |

Setting both forms for one credential is a validation error, not a precedence rule. Precedence rules are where configuration bugs hide: with "file overrides value", an operator setting the env variable and seeing it silently ignored loses an afternoon. An error at startup costs a second.

Files are resolved during config load, before validation, so everything past the config package sees only resolved values and has no idea files were involved. Locally, secret files live in the gitignored `secrets/` directory with mode 600 (owner read/write only).

## Consequences

- Multiline credentials work unchanged; a PEM key is just a file.
- The same config file works locally and deployed: container secret mounts and systemd `LoadCredential=` both deliver secrets as files at a path, which is exactly what the `_file` form consumes.
- `make run` needs no exported variables.
- A leaked `config.yaml` reveals file paths, not secrets.
- Log redaction stays simple: startup logging reports which venues are authenticated, never credential values, and the config struct is never dumped wholesale.
