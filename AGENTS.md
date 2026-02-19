# AGENTS.md

Simple guidance for working in this repo.

## Basics
- Prefer `rg` for searching.
- Keep edits minimal and focused.
- Run relevant tests when changes are non-trivial.

## Go
- Keep code gofmt-compliant.
- Avoid unnecessary allocations.

## Commands
- `just test` if unsure which tests to run.
- `go test ./...` for full suite.

## Release Hygiene
- When making user-visible behavior/output changes, bump `internal/app/version.go` (`Version`) in the same change.
