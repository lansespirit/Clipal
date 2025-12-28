# Clipal

![Clipal](assets/clipal.png)

English: [README.md](README.md) | 中文: [README.zh-CN.md](README.zh-CN.md)

A minimal CLI-first LLM API reverse proxy for tools like Claude Code, Codex CLI, and Gemini CLI.

## What it is

Clipal is a lightweight reverse proxy that routes requests to one of multiple upstream API providers based on a simple YAML configuration, with priority ordering and automatic failover.

## Core ideas

- **Minimal**: no UI, no DB, no history — just proxying
- **Transparent**: no message-format conversion; relies on upstream compatibility
- **Portable**: single cross-platform binary
- **Configurable**: YAML configs separated by client type

## Features

- Multiple upstream providers with priority ordering
- Automatic failover (tries the next provider on errors)
- Temporary provider deactivation on auth/quota errors, with auto-reactivation via `reactivate_after`
- Hot reload: changes to `claude-code.yaml` / `codex.yaml` / `gemini.yaml` reload automatically
- Log levels: `debug` / `info` / `warn` / `error`
- Separate client configs:
  - Claude Code (`claude-code.yaml`)
  - Codex CLI (`codex.yaml`)
  - Gemini CLI (`gemini.yaml`)
- macOS / Linux / Windows support

## Architecture

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   Claude Code   │     │    Codex CLI    │     │   Gemini CLI    │
└────────┬────────┘     └────────┬────────┘     └────────┬────────┘
         │                       │                       │
         │ /claudecode           │ /codex                │ /gemini
         │                       │                       │
         └───────────────────────┼───────────────────────┘
                                 │
                                 ▼
┌─────────────────────────────────────────────────────────────────┐
│                         clipal (:3333)                          │
│                                                                 │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │                       HTTP Router                         │  │
│  │  /claudecode/*  →  claude-code.yaml providers             │  │
│  │  /codex/*       →  codex.yaml providers                   │  │
│  │  /gemini/*      →  gemini.yaml providers                  │  │
│  └───────────────────────────────────────────────────────────┘  │
│                              │                                  │
│                              ▼                                  │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │               Endpoint Router (priority + failover)       │  │
│  └───────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
                               │
         ┌─────────────────────┼─────────────────────┐
         ▼                     ▼                     ▼
┌─────────────────┐   ┌─────────────────┐   ┌─────────────────┐
│  API Provider 1 │   │  API Provider 2 │   │  API Provider N │
│  (priority: 1)  │   │  (priority: 2)  │   │  (priority: N)  │
└─────────────────┘   └─────────────────┘   └─────────────────┘
```

### Route prefixes

| Path prefix | Config file | Notes |
|------------|-------------|-------|
| `/claudecode/*` | `~/.clipal/claude-code.yaml` | Claude Code upstreams |
| `/codex/*` | `~/.clipal/codex.yaml` | Codex CLI upstreams |
| `/gemini/*` | `~/.clipal/gemini.yaml` | Gemini CLI upstreams |

## Configuration

### Global config (`config.yaml`)

```yaml
listen_addr: "127.0.0.1"  # default: 127.0.0.1 (local only)
port: 3333                # default: 3333
log_level: "info"         # debug/info/warn/error
reactivate_after: "1h"    # default: 1h; set to 0 to disable auto-deactivate
max_request_body_bytes: 33554432  # default: 32 MiB (request body is buffered for retries)
log_dir: ""               # default: <config-dir>/logs
log_retention_days: 7     # default: 7
log_stdout: true          # default: true
ignore_count_tokens_failover: false # Claude Code: don't failover main chat on count_tokens failures
```

### Client configs (`claude-code.yaml` / `codex.yaml` / `gemini.yaml`)

```yaml
providers:
  - name: "anthropic-direct"
    base_url: "https://api.anthropic.com"
    api_key: "sk-ant-xxx"
    priority: 1
    enabled: true

  - name: "openrouter"
    base_url: "https://openrouter.ai/api"
    api_key: "sk-or-xxx"
    priority: 2
    enabled: true
```

## Provider selection & failover

For each client (claude-code / codex / gemini), Clipal maintains an independent provider list and selects upstreams with these rules:

- Only uses providers with `enabled != false`
- Sorts by `priority` ascending (lower number = higher priority); ties keep YAML order
- Sticky preference: a successful provider becomes the next preferred one
- On request failure, tries the next available provider
- Temporary deactivation:
  - `401/403` auth and `402` billing/quota errors deactivate the provider and move on
  - `429` is inspected; quota/auth-like cases deactivate, otherwise it failovers without deactivation (cooldown up to `1h`)
- Auto-reactivation: deactivated providers are re-enabled after `reactivate_after`
- Hot reload resets the provider set based on the updated config

## Docs

- English: [docs/README.md](docs/README.md)
- 中文: [docs/README.zh-CN.md](docs/README.zh-CN.md)

## Quick start

1) Download a binary from [Releases](https://github.com/lansespirit/Clipal/releases).

2) Make it executable and place it on your `PATH`:

```bash
chmod +x clipal*
./clipal* --version
```

3) Initialize config from templates:

```bash
mkdir -p ~/.clipal
cp examples/config.yaml ~/.clipal/config.yaml
cp examples/claude-code.yaml ~/.clipal/claude-code.yaml
cp examples/codex.yaml ~/.clipal/codex.yaml
cp examples/gemini.yaml ~/.clipal/gemini.yaml
```

4) Edit `~/.clipal/*.yaml` and set `api_key` (and `base_url` if needed).

5) Start and verify health:

```bash
clipal --log-level debug
curl -fsS http://127.0.0.1:3333/health
```

6) Configure your client (Claude Code / Codex CLI / Gemini CLI) below.

## Install

### Option A: prebuilt binaries (recommended)

Download from [Releases](https://github.com/lansespirit/Clipal/releases).

```bash
# macOS / Linux
chmod +x clipal
sudo mv clipal /usr/local/bin/

# Windows
# Put clipal.exe on PATH
```

### Option B: build from source

```bash
git clone https://github.com/lansespirit/Clipal.git
cd Clipal
go build -o clipal ./cmd/clipal
sudo mv clipal /usr/local/bin/
```

## Run

```bash
# Use the default config dir (~/.clipal/)
clipal

# Custom config dir
clipal --config-dir /path/to/config

# Override listen address
clipal --listen-addr 0.0.0.0

# Override port
clipal --port 8080

# Override log level
clipal --log-level debug
```

## Run in background

```bash
# Linux/macOS - quick background run (prefer file logging)
# Set log_stdout: false in ~/.clipal/config.yaml first, then:
nohup clipal >/dev/null 2>&1 &
```

For proper startup on boot (systemd / launchd / Task Scheduler), see:

- [macOS](docs/en/macos.md)
- [Linux](docs/en/linux.md)
- [Windows](docs/en/windows.md)

## Client setup

### Claude Code

Edit `~/.claude/settings.json`:

```json
{
  "env": {
    "ANTHROPIC_AUTH_TOKEN": "any-value",
    "ANTHROPIC_BASE_URL": "http://127.0.0.1:3333/claudecode"
  }
}
```

### Codex CLI

Edit `~/.codex/config.toml`:

```toml
model_provider = "clipal"

[model_providers.clipal]
name = "clipal"
base_url = "http://localhost:3333/codex"
```

### Gemini CLI

```bash
export GEMINI_API_BASE="http://localhost:3333/gemini"
```

## Logging

By default, Clipal logs to stdout and to a daily-rotated log file (retained for 7 days by default).

- Default log dir: `<config-dir>/logs` (e.g. `~/.clipal/logs`)
- Log file: `clipal-YYYY-MM-DD.log`

For quiet background operation, set in `~/.clipal/config.yaml`:

```yaml
log_stdout: false
log_retention_days: 7
# log_dir: ""  # empty means ~/.clipal/logs by default
```

## Project layout

```
clipal/
├── cmd/
│   └── clipal/
│       └── main.go           # entrypoint
├── internal/
│   ├── config/
│   │   └── config.go         # YAML config loading
│   ├── proxy/
│   │   ├── proxy.go          # routing + request building
│   │   └── failover.go       # failover + de/activation logic
│   └── logger/
│       └── logger.go         # logging
├── build/                    # build outputs
├── scripts/
│   └── build.sh              # cross-platform build script
├── go.mod
├── go.sum
└── README.md
```

## Development

```bash
go build -o clipal ./cmd/clipal
go test ./...

# Note: Go writes build/test caches under GOCACHE.
# If you run in a restricted environment, use a temp dir for GOCACHE:
tmp="$(mktemp -d "${TMPDIR:-/tmp}/go-build-cache.XXXXXX)"
GOCACHE="$tmp" go test ./...
rm -rf "$tmp"

./scripts/build.sh
```

### Supported platforms

| OS | Arch | Artifact name |
|----|------|----------------|
| macOS | amd64 | `clipal-darwin-amd64` |
| macOS | arm64 | `clipal-darwin-arm64` |
| Linux | amd64 | `clipal-linux-amd64` |
| Linux | arm64 | `clipal-linux-arm64` |
| Windows | amd64 | `clipal-windows-amd64.exe` |

## License

MIT

