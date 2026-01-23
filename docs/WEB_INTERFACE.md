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
- **Add Provider**: Add new API providers with custom configurations
- **Edit Provider**: Update existing provider settings
- **Delete Provider**: Remove providers you no longer need
- **Enable/Disable**: Toggle providers on/off without deleting them
- **Priority Management**: Providers are ordered by priority (lower number = higher priority)

### 2. Global Settings
Configure system-wide settings:
- **Server Configuration**: Listen address and port
- **Log Level**: Debug, Info, Warn, or Error
- **Provider Failover**: Configure automatic failover behavior
  - Reactivate After: Duration before reactivating failed providers
  - Upstream Idle Timeout: Timeout for idle connections
  - Ignore Count Tokens Failover: Keep context cache warm for Claude Code
- **Logging**: Configure log directory, retention, and output
- **Notifications**: Desktop notification settings

### 3. System Status
Monitor your Clipal instance:
- Version and uptime information
- Configuration directory location
- Per-client provider statistics
- Enabled providers and first enabled provider (from config)

### 4. Configuration Export
Export your entire configuration as JSON for backup or migration purposes.

## Security Notes

- The web interface is localhost-only (enforced). Requests from non-loopback addresses return 403 even if you bind the proxy to 0.0.0.0/::.
- The management API is unauthenticated by design (for local use)
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
