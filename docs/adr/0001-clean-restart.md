# 0001 — Clean restart: wipe main, original architecture

**Status:** accepted (2026-07-02)

## Context

The repository previously held two generations of code: legacy `main` (layered managers, chi web server, portfolio tracking) and `v3` (a partial hexagonal rewrite). Both were early-days work; the owner judged their designs fragile and did not want either used as the basis for the serious platform described in [ROADMAP.md](../ROADMAP.md).

## Decision

`main` was reset to a brand-new orphan root commit with zero history. The architecture is designed fresh from the domain — legacy `main` and `v3` are cautionary references at most, never templates.

History preservation: branches `backup` and `v1` plus tag `legacy-final` (commit `e91ff72`) hold the full legacy history; branch `v3` holds the abandoned rewrite.

## Consequences

- No inherited design debt; every pattern in this repo is deliberate and documented in an ADR.
- Old code remains recoverable but must not be ported wholesale.
- `_local/v3-leftovers/` holds untracked v3-era working files (old plans, local config); gitignored.
