# Getting Started

## 1. Download and Install

Download the right binary from [Releases](https://github.com/lansespirit/Clipal/releases) and place it on your `PATH`.
Current stable release: [`v0.11.4`](https://github.com/lansespirit/Clipal/releases/tag/v0.11.4)

Platform-specific notes:

- [macOS](macos.md)
- [Linux](linux.md)
- [Windows](windows.md)

Verify the version:

```bash
clipal --version
```

## 2. Initialize Config

Default config directory:

- macOS / Linux: `~/.clipal/`
- Windows: `%USERPROFILE%\\.clipal\\`

Initialize from the repo templates:

```bash
mkdir -p ~/.clipal
cp examples/config.yaml ~/.clipal/config.yaml
cp examples/claude.yaml ~/.clipal/claude.yaml
cp examples/openai.yaml ~/.clipal/openai.yaml
cp examples/gemini.yaml ~/.clipal/gemini.yaml
```

Direct template links:

- [../../examples/config.yaml](../../examples/config.yaml)
- [../../examples/claude.yaml](../../examples/claude.yaml)
- [../../examples/openai.yaml](../../examples/openai.yaml)
- [../../examples/gemini.yaml](../../examples/gemini.yaml)

Then edit those files and fill in your upstream `base_url`, `api_key`, or `api_keys`.

For field details, see [Config Reference](config-reference.md).

## 3. Start Clipal

```bash
clipal
```

Common startup overrides:

```bash
clipal --config-dir /path/to/config
clipal --listen-addr 127.0.0.1
clipal --port 3333
clipal --log-level debug
```

By default, Clipal starts both:

- the local proxy
- the localhost Web management UI

## 4. Verify It Is Running

```bash
curl -fsS http://127.0.0.1:3333/health
clipal status
clipal status --json
```

Open this in your browser:

```text
http://127.0.0.1:3333/
```

## 5. Connect Your Client

Clipal standardizes client ingress on:

- `/clipal`

Compatibility aliases remain available for older setups:

- `/claudecode`
- `/codex`
- `/gemini`

See [Client Setup](client-setup.md) for exact client-side configuration.

## 6. What To Read Next

- Want to manage providers in a GUI: [Web UI Guide](web-ui.md)
- Want to understand failover, pinning, and multi-key behavior: [Routing and Failover](routing-and-failover.md)
- Want autostart or background service setup: [Services, Status, and Updates](services.md)
- Hit a problem: [Troubleshooting](troubleshooting.md)
