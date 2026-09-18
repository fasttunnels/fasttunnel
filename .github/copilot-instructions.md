# Project Guidelines — FastTunnel CLI (Go) — Reference

## Role In This Workspace

- This folder is the existing **FastTunnel** CLI implementation.
- It is included as a **reference implementation** for CLI parsing/dispatch, dependency wiring, and telemetry patterns.
- For the deployment platform architecture/spec, see `deployment/engineering-standards.md` and `deployment/architecture.md`.

## Architecture

- `cmd/fasttunnel/main.go` is wiring + dispatch only.
- Argument/flag parsing lives in `internal/cmdparse/`.
- Command implementations live in `internal/commands/`.
- Keep the CLI composition-first: inject clients/services into commands; avoid globals.

## Composition Over Inheritance (Project-wide rule)

- Prefer functions and small structs with explicit constructor injection.
- Avoid inheritance-like patterns and global mutable state.

## Code Style

- `gofmt` is mandatory.
- Errors: wrap with `%w` and include operation context.
- Keep user-facing errors readable; log structured details via `internal/telemetry`.

## Build & Test

- Build: `go build -o fasttunnel ./cmd/fasttunnel`
- Unit tests: `go test ./...`
- Lint: `golangci-lint run` (uses `.golangci.yml`)
