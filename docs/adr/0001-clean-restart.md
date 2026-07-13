# 0001: Clean restart: wipe main, original architecture

**Status:** accepted (2026-07-02)

## Background: the rewrite question

Every long-lived codebase eventually faces the same decision: the current design is fighting you, so do you refactor it incrementally or start over? The industry default answer is "never rewrite", popularized by the failures of big-bang rewrites that ran for years while the old system kept shipping. That default is worth taking seriously, and this ADR documents why this project went against it anyway, so a future reader can judge whether the reasoning held up.

This repository held two earlier generations of code:

| Generation | What it was | Why it was judged unsuitable |
|---|---|---|
| legacy `main` | layered managers, a chi web server, portfolio tracking | design grew by accretion; gocryptotrader types leaked through every layer, so its breaking changes rippled through the whole program (this pain is what later produced ADR-0003) |
| `v3` | a partial hexagonal rewrite | abandoned mid-flight; carried over enough legacy assumptions that finishing it meant relitigating them one by one |

The standard argument against rewriting is that the old code encodes years of accumulated bug fixes and edge-case knowledge you will lose. That argument assumes the old code is running in production and depended on. Here it was not: nothing depended on the legacy system running, there were no users to migrate, and the accumulated knowledge worth keeping was small enough to carry over as written decisions (these ADRs) rather than as code.

What the decay looked like concretely, so the lesson is more than an adjective: in legacy `main`, gocryptotrader's `exchange.IBotExchange` and its order types appeared in function signatures from the HTTP handlers down to the portfolio math. Upgrading GCT meant compile errors in a dozen packages that had nothing to do with exchanges. Testing portfolio logic meant constructing GCT engine objects. Adding a second data source meant teaching every layer about a second vendor. None of these are bugs; they are the compounding tax of a missing boundary, and the tax grows with every feature. The specific cure is ADR-0003; the general cure is starting from seams instead of retrofitting them.

## Decision

`main` was reset to a brand-new orphan root commit. An orphan commit is a commit with no parent: the branch's history starts there, so `git log` on `main` shows nothing older. The architecture is designed fresh from the domain outward. The legacy generations are reference material for "what did we try before and why did it hurt", never templates to copy from.

Nothing was deleted. Git makes it cheap to keep everything reachable without keeping it in the way:

| Ref | Contents |
|---|---|
| branch `backup`, branch `v1`, tag `legacy-final` (commit `e91ff72`) | the complete legacy history |
| branch `v3` | the abandoned rewrite |
| `_local/v3-leftovers/` (gitignored) | untracked v3-era working files: old plans, local config |

## The rule that makes this safe

A clean restart only stays clean if old patterns do not leak back in through habit. So the rule, recorded here and repeated in AGENTS.md: legacy `main` and `v3` may be read to answer "how did we handle X before and what went wrong", and must never be ported from wholesale. If a legacy approach turns out to be right, it re-enters the codebase through a fresh design and, when significant, its own ADR, so the reasoning is recorded this time.

Practical archaeology, when you need it:

```sh
git log --oneline legacy-final -- path/of/interest   # what happened to a file, historically
git show legacy-final:internal/somepkg/file.go       # read one old file without checking anything out
git diff v3 legacy-final -- path/                    # how the two generations differed on a topic
```

Reading is encouraged; `git checkout legacy-final -- path/` into the working tree is the move this rule forbids.

## Consequences

- Every pattern in this repository is a decision someone made on purpose, and the significant ones have an ADR. A reader never has to wonder whether something is intentional or archaeological.
- The cost: early milestones rebuild things the legacy code already had (exchange connectivity, config, storage). The bet is that rebuilding on clean seams is cheaper than dragging the old coupling forward. M1 and M2 are the test of that bet.
- Old code remains one `git checkout` away if a specific behavior ever needs to be consulted.
