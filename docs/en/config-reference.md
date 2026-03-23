# Config Reference

## Config File Location

Default directory:

- macOS / Linux: `~/.clipal/`
- Windows: `%USERPROFILE%\\.clipal\\`

Default files:

```text
config.yaml
claude-code.yaml
codex.yaml
gemini.yaml
```

`config.yaml` is global config. The other three files define separate client groups.

Matching templates:

- [../../examples/config.yaml](../../examples/config.yaml)
- [../../examples/claude-code.yaml](../../examples/claude-code.yaml)
- [../../examples/codex.yaml](../../examples/codex.yaml)
- [../../examples/gemini.yaml](../../examples/gemini.yaml)

## Minimal Example

`config.yaml`:

```yaml
listen_addr: 127.0.0.1
port: 3333
log_level: info
reactivate_after: 1h
```

`codex.yaml`:

```yaml
mode: auto
pinned_provider: ""

providers:
  - name: openai-compatible
    base_url: https://api.openai.com
    api_key: sk-xxx
    priority: 1
    enabled: true
```

## Global Config `config.yaml`

| Field | Type | Default | Notes |
|-------|------|---------|-------|
| `listen_addr` | string | `127.0.0.1` | Listen address |
| `port` | int | `3333` | Local proxy port |
| `log_level` | string | `info` | `debug` / `info` / `warn` / `error` |
| `reactivate_after` | duration | `1h` | Auto-reactivation delay for temporarily deactivated providers; set `0` to disable temporary deactivation for auth, billing, and quota failures |
| `upstream_idle_timeout` | duration | `3m` | Abort the current upstream attempt if no response body bytes arrive for too long |
| `response_header_timeout` | duration | `2m` | Timeout while waiting for upstream response headers |
| `max_request_body_bytes` | int | `33554432` | Request body size limit, default 32 MiB |
| `log_dir` | string | `<config-dir>/logs` | Log directory |
| `log_retention_days` | int | `7` | Log retention days; `0` keeps logs forever; default is 7 days |
| `log_stdout` | bool | `true` | Also log to stdout; long-running background setups usually prefer `false` |

### `notifications`

```yaml
notifications:
  enabled: false
  min_level: error
  provider_switch: true
```

| Field | Type | Default | Notes |
|-------|------|---------|-------|
| `enabled` | bool | `false` | Enable desktop notifications |
| `min_level` | string | `error` | `debug` / `info` / `warn` / `error` |
| `provider_switch` | bool | `true` | Send notifications on provider switches |

### `circuit_breaker`

```yaml
circuit_breaker:
  failure_threshold: 4
  success_threshold: 2
  open_timeout: 60s
  half_open_max_inflight: 1
```

| Field | Type | Default | Notes |
|-------|------|---------|-------|
| `failure_threshold` | int | `4` | Open the circuit after this many consecutive failures; `0` disables it |
| `success_threshold` | int | `2` | Consecutive half-open successes needed to close the circuit |
| `open_timeout` | duration | `60s` | How long the circuit stays open before probing again |
| `half_open_max_inflight` | int | `1` | Max concurrent half-open probes |

### `routing`

```yaml
routing:
  sticky_sessions:
    enabled: true
    explicit_ttl: 30m
    cache_hint_ttl: 10m
    dynamic_feature_ttl: 10m
    dynamic_feature_capacity: 1024
    response_lookup_ttl: 15m
  busy_backpressure:
    enabled: true
    retry_delays:
      - 5s
      - 10s
    probe_max_inflight: 1
    short_retry_after_max: 3s
    max_inline_wait: 8s
```

`sticky_sessions` controls how long Clipal keeps affinity hints in memory:

- `explicit_ttl`: durable linkage keys such as OpenAI `previous_response_id`
- `cache_hint_ttl`: cache-oriented hints such as `prompt_cache_key`
- `dynamic_feature_ttl`: short-lived heuristic bindings derived from human-message history
- `dynamic_feature_capacity`: max in-memory dynamic/cache-level entries before least-recently-used eviction
- `response_lookup_ttl`: response-id lookup cache lifetime

`busy_backpressure` controls how Clipal reacts to concurrency-limit `429` responses:

- `retry_delays`: inline wait/backoff schedule before overflow
- `probe_max_inflight`: max concurrent recovery probes allowed per busy provider
- `short_retry_after_max`: only very short retry hints are eligible for busy handling
- `max_inline_wait`: hard cap for how long Clipal holds one request before overflowing to another provider

## Client Configs

All three client files share the same structure:

- `claude-code.yaml`
- `codex.yaml`
- `gemini.yaml`

Example:

```yaml
mode: auto
pinned_provider: ""

providers:
  - name: primary
    base_url: https://api.example.com
    api_key: sk-xxx
    priority: 1
    enabled: true

  - name: backup
    base_url: https://backup.example.com
    api_keys:
      - sk-a
      - sk-b
    priority: 2
    enabled: true
```

### Top-Level Fields

| Field | Type | Default | Notes |
|-------|------|---------|-------|
| `mode` | string | `auto` | `auto` or `manual` |
| `pinned_provider` | string | empty | Provider name to lock to when `mode: manual` |
| `providers` | array | none | Provider list |

### `providers[]`

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `name` | string | yes | Provider name |
| `base_url` | string | yes | Upstream API base URL |
| `api_key` | string | one of two | Single API key |
| `api_keys` | array | one of two | Multiple API keys, used in order |
| `priority` | int | no | Lower number = higher priority; omitted or `0` is treated as `1` |
| `enabled` | bool | no | Defaults to `true` |

## Practical Defaults

- Use `api_key` when you only have one key
- Use `api_keys` when you want retries across multiple keys within the same provider
- For long-running background setups, this is a good default:

```yaml
log_stdout: false
log_retention_days: 7 # 0 keeps logs forever
```

- For safety, keep this unless you explicitly need network exposure:

```yaml
listen_addr: 127.0.0.1
```

## Related Docs

- [Routing and Failover](routing-and-failover.md)
- [Web UI Guide](web-ui.md)
- [Services, Status, and Updates](services.md)
