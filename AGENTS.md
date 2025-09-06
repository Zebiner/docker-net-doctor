# Repository Guidelines

## Project Structure & Module Organization
- `cmd/docker-net-doctor/`: CLI entrypoint (`main.go`).
- `internal/`: application packages
  - `docker/`: Docker SDK wrapper and helpers.
  - `diagnostics/`: diagnostic engine and checks.
  - `cli/`, `analyzer/`: user I/O and analysis utilities.
- `pkg/`: public utility packages (e.g., `pkg/netutils`).
- `test/`: fixtures and integration tests (`test/integration`).
- `bin/`: built binaries (git-ignored).
- `docs/`: usage examples and reference.
- `scripts/`: helper scripts (e.g., `setup-deps.sh`).

## Build, Test, and Development Commands
- `make build`: compile the CLI to `bin/docker-net-doctor` with version info.
- `make test`: run unit tests with race detector and coverage (`coverage.out`).
- `go tool cover -func=coverage.out`: show coverage summary.
- `make test-integration`: spin up test compose stack and run integration tests.
- `make install`: install as user Docker CLI plugin (`~/.docker/cli-plugins`).
- `make install-system`: install system-wide (requires sudo).
- `make fmt` / `make lint`: format via `gofmt` and lint via `golangci-lint`.
- `make dev`: rebuild on file changes (requires `entr`).

## Coding Style & Naming Conventions
- Go 1.x modules; use standard Go formatting (`gofmt -s`).
- Prefer package-scoped, descriptive names; avoid stutter (e.g., `docker.Client`).
- Files: tests end with `_test.go`; examples go under `docs/examples/`.
- Keep functions small; return errors with context (`fmt.Errorf("...: %w", err)`).
- Run `make fmt lint` before submitting.

## Testing Guidelines
- Framework: `go test` with `testify` where helpful.
- Unit tests under the same package; integration under `test/integration` (may require Docker).
- Name tests `TestXxx` and table-test where sensible.
- Use `t.Skip` when Docker daemon is unavailable (see existing tests).

## Commit & Pull Request Guidelines
- Commits: concise, imperative subject (<=72 chars), body explains what/why.
- Reference issues with `Fixes #id`/`Refs #id` when applicable.
- PRs: include summary, motivation, screenshots/logs if UX/CLI output changes, and testing notes.
- Ensure CI passes locally: `make test lint` and, if relevant, `make test-integration`.

## Security & Configuration Tips
- Docker access is required for many tests; prefer least-privilege contexts.
- Avoid committing secrets; use environment variables and `.gitignore`d files.
- When adding diagnostics that execute commands in containers, validate input and handle timeouts.

