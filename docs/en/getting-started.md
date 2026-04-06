# Getting Started

## 1. Download and Install

Download the right binary from [Releases](https://github.com/lansespirit/Clipal/releases) and place it on your `PATH`.
Latest stable release: [GitHub Releases latest](https://github.com/lansespirit/Clipal/releases/latest)

Platform-specific notes:

- [macOS](macos.md)
- [Linux](linux.md)
- [Windows](windows.md)

Verify the version:

```bash
clipal --version
```

## 2. Start Clipal

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
- the Web management UI at `http://127.0.0.1:3333/`

## 3. Configure Providers via Web UI

Open the Web UI in your browser and add your providers there — no config files needed:

```text
http://127.0.0.1:3333/
```

From the **Providers** page you can:

- Add, edit, or remove providers (Claude, OpenAI, Gemini, and any OpenAI-compatible endpoint)
- Set `base_url`, `api_key` / `api_keys`, and routing weights
- Enable or disable individual keys without restarting

Changes take effect immediately without a restart.

> **Advanced / Optional — Manual YAML config**
>
> If you prefer to manage configuration as code, you can still edit YAML files directly.
> Default config directory:
> - macOS / Linux: `~/.clipal/`
> - Windows: `%USERPROFILE%\\.clipal\\`
>
> Copy the example templates to get started:
>
> ```bash
> mkdir -p ~/.clipal
> cp examples/config.yaml ~/.clipal/config.yaml
> cp examples/claude.yaml ~/.clipal/claude.yaml
> cp examples/openai.yaml ~/.clipal/openai.yaml
> cp examples/gemini.yaml ~/.clipal/gemini.yaml
> ```
>
> Template links: [config.yaml](../../examples/config.yaml) · [claude.yaml](../../examples/claude.yaml) · [openai.yaml](../../examples/openai.yaml) · [gemini.yaml](../../examples/gemini.yaml)
>
> For field details, see [Config Reference](config-reference.md).

## 4. Verify It Is Running

```bash
curl -fsS http://127.0.0.1:3333/health
clipal status
clipal status --json
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

- Want a full walkthrough of the Web UI: [Web UI Guide](web-ui.md)
- Want to understand failover, pinning, and multi-key behavior: [Routing and Failover](routing-and-failover.md)
- Want autostart or background service setup: [Services, Status, and Updates](services.md)
- Hit a problem: [Troubleshooting](troubleshooting.md)
