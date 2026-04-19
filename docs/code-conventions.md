# Code Conventions

## Core rules

- Use `decimal.Decimal` for monetary values.
- Do not use `float64` for money.
- Use `context.Context` as the first parameter for I/O operations.
- Prefer `clock.Clock` over direct `time.Now()` in test-sensitive logic.
- Do not panic in library code; return errors instead.
- Use Fx for module wiring instead of ad-hoc manual composition in production paths.

## Naming

### Packages
- short, lowercase, single-word names
- package name should match directory name

### Terminology
Prefer the canonical repo vocabulary:
- `storage` = persistence ports
- `adapters` = infrastructure implementations
- `app` = orchestration / use-case layer
- `core` = runtime/bootstrap composition only
- `holding` = asset balance object in the portfolio domain

Treat older words like `repository`, `platform`, `service`, and `position` as historical unless a file must reference legacy context explicitly.

## Error handling

- Wrap with useful context via `%w`
- Keep sentinel errors in `internal/errs/`
- Use `errors.Is()` / `errors.As()` for checks

Example:

```go
result, err := doThing(ctx)
if err != nil {
    return fmt.Errorf("failed to do thing: %w", err)
}
```

## Imports

Order imports as:
1. standard library
2. third-party packages
3. internal packages

Use `goimports` formatting where practical.

## Comments and docs

- Exported symbols should have doc comments.
- Use plain English in comments.
- Avoid Unicode-heavy LLM-style punctuation in code comments.
- Explain why when the code is non-obvious, not what the code literally says.

## Interface design

- Accept interfaces, return structs where practical.
- Prefer small interfaces by behavior.
- Keep storage boundaries split by capability instead of giant interfaces.

## Execution discipline for changes

- Create dependencies before dependents.
- Define types before implementation using them.
- Add config before code that consumes it.
- Replace old code before deleting old code.
- Verify package-level compile/test checkpoints during larger changes.
- Prefer walking-skeleton validation for new external integrations.

## Probes and learning commands

- `cmd/devprobe-*` and `cmd/learn/*` are allowed in-repo.
- They are intentionally non-production helpers.
- Generated artifacts from them must not be committed.
- Keep TLS material, local configs, logs, and other runtime byproducts out of git.
