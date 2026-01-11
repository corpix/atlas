# Repository Guidelines

## Project Structure & Module Organization
This repository is a Go library organized by package. Each top-level directory is a package focused on a specific concern, for example `dump`, `errors`, `log`, `pool`, `queue`, `rpc` (with `rpc/auth` for auth helpers), `seq`, `sqlite`, `postgres`, `supervisor`, and `watcher`. Source lives alongside its tests, so look for `*_test.go` files next to the implementation. Build and tooling configuration lives at the repo root in `go.mod`, `go.sum`, `makefile`, and optional Nix files (`flake.nix`, `flake.lock`).

## Build, Test, and Development Commands
- `make test` runs `make lint` and then `go test -v ./...`.
- `make lint` runs `go vet ./...` and `golangci-lint run -v`.
- `make fmt` runs `go fmt ./...` and `fieldalignment -fix` for this module.
- `go test ./...` is a faster local check when you do not need linting.
- `make tag` creates a version tag based on date and commit count.

## Coding Style & Naming Conventions
Stick to standard Go formatting and layout. Use `gofmt` (via `go fmt ./...`) and keep imports organized. Package names are lower-case and correspond to their directory names (for example `rpc`, `sqlite`). Test files should remain adjacent to code and follow `*_test.go` naming. Keep exported identifiers in `CamelCase` and unexported in `camelCase`, consistent with Go conventions.

## Testing Guidelines
Tests use the Go testing framework, with dependencies such as `testify` and `pgxmock` where needed. Name test functions `TestXxx` and keep behavior-focused tests near the package they validate. Run all tests with `make test`; run a package-only test with `go test ./sqlite` or similar.

## Commit & Pull Request Guidelines
Recent history uses a concise `area: description` style (for example `dump: support diff options`, `supervisor: simplify interface`). Follow that pattern for new commits. For pull requests, include a short summary, the main packages affected, and the exact test command(s) you ran. Link related issues when applicable.
