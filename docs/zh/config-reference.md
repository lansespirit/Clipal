# 配置参考

## 配置文件位置

默认目录：

- macOS / Linux: `~/.clipal/`
- Windows: `%USERPROFILE%\\.clipal\\`

默认文件：

```text
config.yaml
claude-code.yaml
codex.yaml
gemini.yaml
```

`config.yaml` 是全局配置，其余三个文件分别管理不同客户端分组。

对应模板：

- [../../examples/config.yaml](../../examples/config.yaml)
- [../../examples/claude-code.yaml](../../examples/claude-code.yaml)
- [../../examples/codex.yaml](../../examples/codex.yaml)
- [../../examples/gemini.yaml](../../examples/gemini.yaml)

## 最小示例

`config.yaml`：

```yaml
listen_addr: 127.0.0.1
port: 3333
log_level: info
reactivate_after: 1h
```

`codex.yaml`：

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

## 全局配置 `config.yaml`

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `listen_addr` | string | `127.0.0.1` | 监听地址 |
| `port` | int | `3333` | 本地代理端口 |
| `log_level` | string | `info` | `debug` / `info` / `warn` / `error` |
| `reactivate_after` | duration | `1h` | provider 临时禁用后的自动恢复时间；设为 `0` 可禁用基于鉴权、计费、额度错误的临时禁用 |
| `upstream_idle_timeout` | duration | `3m` | 上游响应 body 长时间无字节时中断当前尝试 |
| `response_header_timeout` | duration | `2m` | 等待上游响应头的超时 |
| `max_request_body_bytes` | int | `33554432` | 请求体大小上限，默认 32 MiB |
| `log_dir` | string | `<config-dir>/logs` | 日志目录 |
| `log_retention_days` | int | `7` | 日志保留天数；`0` 表示永久保留；默认保留 7 天 |
| `log_stdout` | bool | `true` | 是否同时输出到 stdout；长期后台运行通常建议设为 `false` |

### `notifications`

```yaml
notifications:
  enabled: false
  min_level: error
  provider_switch: true
```

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `enabled` | bool | `false` | 是否启用桌面通知 |
| `min_level` | string | `error` | `debug` / `info` / `warn` / `error` |
| `provider_switch` | bool | `true` | 是否为 provider 切换发送通知 |

### `circuit_breaker`

```yaml
circuit_breaker:
  failure_threshold: 4
  success_threshold: 2
  open_timeout: 60s
  half_open_max_inflight: 1
```

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `failure_threshold` | int | `4` | 连续失败多少次后打开熔断；`0` 表示禁用 |
| `success_threshold` | int | `2` | 半开状态下连续成功多少次后恢复 |
| `open_timeout` | duration | `60s` | 熔断打开多久后进入半开探测 |
| `half_open_max_inflight` | int | `1` | 半开探测并发上限 |

## 客户端配置

三个客户端文件结构相同：

- `claude-code.yaml`
- `codex.yaml`
- `gemini.yaml`

示例：

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

### 顶层字段

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `mode` | string | `auto` | `auto` 或 `manual` |
| `pinned_provider` | string | 空 | `mode: manual` 时要锁定的 provider 名称 |
| `providers` | array | 无 | provider 列表 |

### `providers[]`

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `name` | string | 是 | provider 名称 |
| `base_url` | string | 是 | 上游 API Base URL |
| `api_key` | string | 二选一 | 单个 API Key |
| `api_keys` | array | 二选一 | 多个 API Key，按顺序使用 |
| `priority` | int | 否 | 数字越小优先级越高；省略或 `0` 时按 `1` 处理 |
| `enabled` | bool | 否 | 是否启用，默认 `true` |

## 使用建议

- 只有一个 key 时用 `api_key`
- 需要同 provider 多 key 轮转时用 `api_keys`
- 常驻后台运行时，建议：

```yaml
log_stdout: false
log_retention_days: 7 # 0 表示永久保留
```

- 面向局域网暴露代理前，请先明确安全边界；默认建议保持：

```yaml
listen_addr: 127.0.0.1
```

## 相关文档

- [路由与故障切换](routing-and-failover.md)
- [Web UI 使用说明](web-ui.md)
- [后台服务、状态与更新](services.md)
