# Contributing

## Toolchain

- Go version: use the version pinned in [`go.mod`](go.mod)
- Lint: `golangci-lint`
- Security scan: `govulncheck`

## Common Commands

```bash
make test-unit
make test-smoke
make lint
make vuln
make ci
```

## Code Organization

- `cmd/clipal`: CLI entrypoints only
- `internal/app`: application bootstrap and runtime assembly
- `internal/config`: config loading, normalization, validation
- `internal/proxy`: request routing and failover runtime
- `internal/web`: localhost-only management UI and HTTP API
- `internal/service`: OS background service planning/execution

## Dependency Rules

- `cmd/clipal/main.go` assembles runtime through `internal/app`
- `internal/config` must stay independent from runtime packages
- `internal/service` must not depend on `internal/proxy`, `internal/web`, or `internal/app`

These rules are enforced by [`.golangci.yml`](.golangci.yml).

## Before Opening a PR

```bash
make test-unit
make lint
make vuln
make test-smoke
```

If you change configuration handling or startup behavior, also verify:

- `clipal status`
- `clipal service status`
- Web UI config save/reload flows
