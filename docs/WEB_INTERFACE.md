# Web Management Interface

Clipal now includes a built-in web management interface for easy configuration management.

## Accessing the Interface

Once Clipal is running, access the web interface at:

```
http://127.0.0.1:3333/
```

(Replace the port if you've configured a different one)

## Features

### 1. Provider Management
- **View Providers**: See all configured providers for each client type (Claude Code, Codex, Gemini)
- **Mode & Pin**: Switch per-client mode (`auto` / `manual`) and pin a provider for manual routing
- **One-click 📌 Pin**: Pin a provider from the provider list (sets mode to `manual` + updates `pinned_provider`)
- **One-click Unpin/Auto**: Switch back to `auto` failover mode from the pinned section
- **Add Provider**: Add new API providers with custom configurations
- **Edit Provider**: Update existing provider settings
- **Delete Provider**: Remove providers you no longer need
- **Enable/Disable**: Toggle providers on/off without deleting them
- **Priority Management**: Providers are ordered by priority (lower number = higher priority; priorities start at 1)
  - When a client is in `manual` mode, the pinned provider cannot be disabled

### 2. Global Settings
Configure system-wide settings:
- **Server Configuration**: Listen address and port
- **Log Level**: Debug, Info, Warn, or Error
- **Provider Failover**: Configure automatic failover behavior
  - Reactivate After: Duration before reactivating failed providers
  - Upstream Idle Timeout: Timeout for idle connections
  - Ignore Count Tokens Failover: Keep context cache warm for Claude Code
- **Circuit Breaker**: Configure circuit breaker thresholds and open timeout
  - Set **Failure Threshold** to `0` to disable the circuit breaker
- **Logging**: Configure log directory, retention, and output
- **Notifications**: Desktop notification settings

### 3. System Status
Monitor your Clipal instance:
- Version and uptime information
- Configuration directory location
- Per-client routing info (mode / pinned / current)
- Last provider switch event (when in auto mode and a failover occurs)
- Per-provider runtime status and skip reason (used by auto routing):
  - `disabled` (disabled in config)
  - `deactivated` (temporary cooldown; shows remaining time)
  - `circuit_open` (circuit breaker open; shows remaining time)

### 4. Service Management
Manage Clipal as an OS background service (same as `clipal service *`):
- Install / uninstall the system service
- Start / stop / restart
- View `status` output (best-effort; may vary by OS)

### 5. Configuration Export
Export your entire configuration as JSON for backup or migration purposes.

## Security Notes

- The web interface is localhost-only (enforced). Requests from non-loopback addresses return 403 even if you bind the proxy to 0.0.0.0/::.
- The UI additionally enforces a localhost Host header (localhost/127.0.0.1/[::1]) to mitigate DNS rebinding attacks.
- The management API is unauthenticated by design (for local use)
- State-changing API requests require `X-Clipal-UI: 1` and `Content-Type: application/json` (the bundled UI sets these automatically; include them if you're calling the API manually).
- WebUI API request bodies are capped at 1 MiB.
- API keys are never displayed in the interface (shown as ••••••••)
- All configuration changes are validated before being saved
- Configuration files are saved with 0600 permissions (owner read/write only) and written atomically to avoid partial reads during hot reload

### Provider Name Rules

Provider names are used as URL identifiers. Allowed format:

- 1-64 characters
- Must start with a letter or number
- Allowed characters: letters, numbers, `.`, `_`, `-`
- Not allowed: `/` or `\`

## Configuration Hot Reload

Changes made through the web interface are automatically detected by Clipal's configuration watcher. Most runtime settings are applied within 5 seconds without requiring a restart (including provider lists, log level, failover timings, request body limit, and notification settings).

Note: Changes to listen address/port and log file output settings may require a restart to take effect.

## Technology Stack

The web interface is built with:
- **Backend**: Go standard library (net/http)
- **Frontend**: Alpine.js (vendored) + Vanilla CSS + Vanilla JavaScript
- **Deployment**: Embedded static files (single binary)

No external dependencies or build steps required!
