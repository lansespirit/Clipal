# Web UI Guide

## How To Access It

Once Clipal is running, open:

```text
http://127.0.0.1:3333/
```

If you changed the port, replace `3333` with your actual port.

## What The Web UI Can Do

### Providers

- View providers by client group
- Add, edit, and delete providers
- Enable or disable providers
- Switch between `auto` and `manual`
- Pin a provider in `manual` mode
- Enter either a single `api_key` or multi-line `api_keys`

### Global Settings

- Change listen address and port
- Change log level
- Configure reactivation, timeouts, and request body limits
- Configure the circuit breaker
- Configure log directory, retention, and stdout output
- Configure desktop notifications

### System Status

- View version and uptime
- View config directory
- View current mode, pinned provider, and preferred provider per client group
- View last switch event and last request summary
- View provider runtime state, configured key count, and available key count

### Services

- Use the common `clipal service` actions from the UI
- Install, start, stop, restart, or uninstall the background service
- View best-effort service status

### Export

- Export the current config as JSON for backup or migration

## Common Provider States In The UI

- `disabled`: manually disabled in config
- `deactivated`: temporarily skipped because of auth, quota, or cooldown logic
- `circuit_open`: blocked by the circuit breaker
- `keys_exhausted`: no currently available API keys for that provider

## Security Boundary

- The Web UI is localhost-only
- Even if the proxy listens on `0.0.0.0` or `::`, the management UI rejects non-loopback requests
- The management API is intended for local use and does not add a separate auth layer
- State-changing API calls require `X-Clipal-UI: 1`
- State-changing calls with a body require `Content-Type: application/json`
- The UI never shows raw API keys directly

## Which Changes Usually Apply Immediately

Most config edits are hot-reloaded without a restart, including:

- provider lists
- priorities
- `mode`
- failover-related timings
- request body limit
- notification settings

Changes that often still need a restart to fully take effect:

- listen address
- listen port
- some log output target changes

## Related Docs

- [Config Reference](config-reference.md)
- [Routing and Failover](routing-and-failover.md)
- [Services, Status, and Updates](services.md)
