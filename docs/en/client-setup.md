# Client Setup

## One Important Mental Model

Clipal does not route by app name. It routes by request style, and `/clipal` is the preferred ingress for all of them:

- Claude-style
- OpenAI / Codex-style
- Gemini-style

Pick the client base URL that points at `/clipal`. Clipal detects the request family from the upstream path. The older `/claudecode`, `/codex`, and `/gemini` prefixes are still available as compatibility aliases.

One important routing detail: under `/clipal`, generic `/v1/*` resource paths follow OpenAI-compatible routing by default. Gemini-specific routing is kept on `/v1beta/*`, `/upload/*`, and Gemini model RPC paths such as `/v1beta/models/{model}:generateContent`.

## Claude Code

Edit `~/.claude/settings.json`:

```json
{
  "env": {
    "ANTHROPIC_AUTH_TOKEN": "any-value",
    "ANTHROPIC_BASE_URL": "http://127.0.0.1:3333/clipal"
  }
}
```

Notes:

- `ANTHROPIC_AUTH_TOKEN` can usually be any non-empty placeholder value
- Clipal replaces upstream auth with the provider credentials from local config

## Codex CLI

Edit `~/.codex/config.toml`:

```toml
model_provider = "clipal"

[model_providers.clipal]
name = "clipal"
base_url = "http://127.0.0.1:3333/clipal"
```

## Gemini CLI

```bash
export GEMINI_API_BASE="http://127.0.0.1:3333/clipal"
```

## Generic OpenAI-Compatible Clients

For local clients that support a custom OpenAI Base URL or API host, the usual choice is:

```text
Base URL: http://127.0.0.1:3333/clipal
```

Common examples:

- Cherry Studio
- Kelivo
- Chatbox
- ChatWise
- other desktop apps with OpenAI-compatible mode

Typical settings:

- Provider type: OpenAI Compatible / OpenAI API
- Base URL: `http://127.0.0.1:3333/clipal`
- API Key: if the client insists, any non-empty placeholder usually works

Notes:

- Compatibility still depends on the exact paths, payload format, and model parameters the client sends
- If you are migrating an existing setup, the legacy aliases remain available: `/claudecode`, `/codex`, `/gemini`

## Quick Checks

- Clipal is running: `clipal status`
- Health check works: `curl -fsS http://127.0.0.1:3333/health`
- The client points to `http://127.0.0.1:3333/clipal`
- The client is not still pointing at an old official API host somewhere else

If setup still fails, continue with [Troubleshooting](troubleshooting.md).
