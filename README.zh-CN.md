# Clipal

![Clipal](assets/clipal.png)

English: [README.md](README.md) | 中文: [README.zh-CN.md](README.zh-CN.md)

极简 CLI LLM API 反向代理服务

## 项目定位

clipal 是一个轻量级的 LLM API 反向代理服务，专为 Claude Code、Codex CLI、Gemini CLI 等命令行工具设计。它通过简单的 YAML 配置文件管理多个 API 供应商，实现自动故障转移和优先级调度。

### 核心理念

- **极简**：无 UI、无数据库、无历史记录，只做代理转发
- **透明**：不转换消息格式，依赖上游 API 供应商的格式兼容能力
- **便携**：单一二进制文件，跨平台运行
- **可配置**：YAML 配置文件，按客户端类型分离

## 功能特性

- 多 API 供应商配置，支持优先级排序
- 自动故障转移：当前供应商失败时自动切换到下一个
- Provider 临时禁用：鉴权/额度错误会自动 deactivate，并按 `reactivate_after` 自动恢复
- 配置热加载：更新 `claude-code.yaml` / `codex.yaml` / `gemini.yaml` 后自动重新加载并重新验证
- 按日志级别输出运行日志（DEBUG/INFO/WARN/ERROR）
- 三套独立配置文件，分别服务于：
  - Claude Code (`claude-code.yaml`)
  - Codex CLI (`codex.yaml`)
  - Gemini CLI (`gemini.yaml`)
- 跨平台支持：macOS、Linux、Windows

## 架构设计

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  Claude Code    │     │   Codex CLI     │     │   Gemini CLI    │
└────────┬────────┘     └────────┬────────┘     └────────┬────────┘
         │                       │                       │
         │ /claudecode           │ /codex                │ /gemini
         │                       │                       │
         └───────────────────────┼───────────────────────┘
                                 │
                                 ▼
┌─────────────────────────────────────────────────────────────────┐
│                    clipal (:3333)                               │
│                                                                 │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │                    HTTP Router                            │  │
│  │  /claudecode/*  →  claude-code.yaml providers             │  │
│  │  /codex/*       →  codex.yaml providers                   │  │
│  │  /gemini/*      →  gemini.yaml providers                  │  │
│  └───────────────────────────────────────────────────────────┘  │
│                              │                                  │
│                              ▼                                  │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │              Endpoint Router (优先级调度)                  │  │
│  │         失败自动切换 → 下一优先级供应商                       │  │
│  └───────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
                               │
         ┌─────────────────────┼─────────────────────┐
         ▼                     ▼                     ▼
┌─────────────────┐   ┌─────────────────┐   ┌─────────────────┐
│  API Provider 1 │   │  API Provider 2 │   │  API Provider N │
│  (优先级: 1)     │   │  (优先级: 2)     │   │  (优先级: N)     │
└─────────────────┘   └─────────────────┘   └─────────────────┘
```

### 端点路由

| 路径前缀 | 对应配置 | 说明 |
|----------|----------|------|
| `/claudecode/*` | `claude-code.yaml` | Claude Code 请求 |
| `/codex/*` | `codex.yaml` | Codex CLI 请求 |
| `/gemini/*` | `gemini.yaml` | Gemini CLI 请求 |

## 配置文件

配置文件位于 `~/.clipal/` 目录下。

### 目录结构

```
~/.clipal/
├── config.yaml             # 全局配置（端口、日志级别）
├── claude-code.yaml        # Claude Code 专用配置
├── codex.yaml              # Codex CLI 专用配置
└── gemini.yaml             # Gemini CLI 专用配置
```

从仓库 `examples/` 拷贝模板（首次使用推荐）：

```bash
mkdir -p ~/.clipal
cp examples/config.yaml ~/.clipal/config.yaml
cp examples/claude-code.yaml ~/.clipal/claude-code.yaml
cp examples/codex.yaml ~/.clipal/codex.yaml
cp examples/gemini.yaml ~/.clipal/gemini.yaml
```

### 全局配置

```yaml
# ~/.clipal/config.yaml
listen_addr: 127.0.0.1    # 监听地址，默认 127.0.0.1（仅本机访问）
port: 3333              # 服务端口，默认 3333
log_level: info         # debug | info | warn | error
reactivate_after: 1h    # Provider 自动恢复间隔，默认 1h（解除临时禁用）
upstream_idle_timeout: 3m # 上游响应 body 长时间无字节则中断该尝试并切换（默认 3m；设为 0 可禁用）
max_request_body_bytes: 33554432 # 请求体大小上限（字节），默认 32 MiB
log_dir: ""             # 日志目录（默认：<config-dir>/logs，例如 ~/.clipal/logs）
log_retention_days: 7   # 日志保留天数（默认 7）
log_stdout: true        # 是否同时输出到 stdout（后台静默运行可设为 false）
ignore_count_tokens_failover: false # Claude Code: count_tokens 失败不影响主会话 provider（保持 context cache）
```

### 客户端配置格式

```yaml
# ~/.clipal/claude-code.yaml
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

  - name: "backup-provider"
    base_url: "https://backup.example.com"
    api_key: "sk-backup-xxx"
    priority: 3
    enabled: false  # 禁用此供应商
```

### 配置字段说明

**全局配置 (config.yaml)**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `listen_addr` | string | 否 | 监听地址，默认 127.0.0.1（仅本机访问） |
| `port` | int | 否 | 代理服务监听端口，默认 3333 |
| `log_level` | string | 否 | 日志级别：debug/info/warn/error，默认 info |
| `reactivate_after` | duration | 否 | Provider 自动恢复间隔（如 `1h`/`30m`），默认 `1h`；设为 `0` 表示不对鉴权/额度错误执行临时禁用 |
| `upstream_idle_timeout` | duration | 否 | 上游响应 body 长时间无字节则中断该尝试并切换到下一个 provider，默认 `3m`；设为 `0` 可禁用 |
| `max_request_body_bytes` | int | 否 | 请求体大小上限（字节）。clipal 会缓存请求体以支持重试，默认 `33554432`（32 MiB） |
| `log_dir` | string | 否 | 日志目录（默认：`<config-dir>/logs`） |
| `log_retention_days` | int | 否 | 日志保留天数（默认 7） |
| `log_stdout` | bool | 否 | 是否同时输出到 stdout（默认 true） |
| `ignore_count_tokens_failover` | bool | 否 | Claude Code：`/v1/messages/count_tokens` 的失败不影响主会话 provider 选择（默认 false） |

**客户端配置 (claude-code.yaml / codex.yaml / gemini.yaml)**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `providers` | array | 是 | API 供应商列表 |
| `providers[].name` | string | 是 | 供应商名称（用于日志标识） |
| `providers[].base_url` | string | 是 | API 供应商 Base URL |
| `providers[].api_key` | string | 是 | API Key |
| `providers[].priority` | int | 是 | 优先级（数字越小优先级越高） |
| `providers[].enabled` | bool | 否 | 是否启用，默认 true |

## Provider 优先级与调度策略

clipal 对每个客户端（claude-code / codex / gemini）独立维护一组 providers，并按以下策略选择上游：

- **启用过滤**：只使用 `enabled != false` 的 providers。
- **优先级排序**：按 `priority` 升序（数字越小优先级越高）；同优先级保持 YAML 文件中的顺序。
- **粘性优先**：每个客户端维护一个 `currentIndex`，默认从优先级最高的 provider 开始；一旦某个 provider 成功响应，会把 `currentIndex` 更新为该 provider，使后续请求优先继续使用它（减少频繁切换）。
- **失败切换**：请求失败时会按顺序切换到下一个可用 provider（直到尝试完所有未被禁用的 provider）。
- **临时禁用（deactivate）**：
  - 鉴权/权限问题（`401/403`）或计费/额度问题（`402`）会将该 provider 标记为临时禁用并切换到下一个。
  - `429` 会进一步解析 OpenAI/Claude 的错误结构：若判断为配额/鉴权类则禁用；若是 rate limit/overload 则仅切换重试不禁用。
- **自动恢复**：被临时禁用的 provider 会在 `reactivate_after` 到期后自动恢复（默认 `1h`）。
- **配置热加载**：当 `claude-code.yaml` / `codex.yaml` / `gemini.yaml` 文件发生变更，会自动重新加载配置并重建 providers（等价于重新验证并清空上一轮的临时禁用状态）。

### 合理性分析（当前定位：本地 CLI 反代）

- **优点**：实现简单、行为可预测；严格优先级 + 失败切换满足“主用/备用”场景；粘性优先减少频繁切换；对鉴权/额度错误自动隔离可避免反复命中坏 key。
- **注意点**：
  - 该策略是“高可用优先”，不是负载均衡；默认会持续偏向当前可用的 provider。
  - 对 `429` 的处理会按 `Retry-After` / `X-RateLimit-Reset-*` 进行冷却（cooldown）并切换到下一个；冷却期内该 provider 会被跳过，避免立刻重复触发限流（最大冷却上限 `1h`）。

## 使用方法

更多平台细节（下载/放置/权限/开机自启/静默运行）见：

- [docs/README.zh-CN.md](docs/README.zh-CN.md)
- [macOS](docs/zh/macos.md) / [Linux](docs/zh/linux.md) / [Windows](docs/zh/windows.md)

### 快速开始（通用）

1) 从 [Releases](https://github.com/lansespirit/Clipal/releases) 下载对应平台的二进制（例如 macOS M 系列：`clipal-darwin-arm64`）。

2) 赋予可执行权限并放到 PATH：

```bash
chmod +x clipal*
./clipal* --version
```

3) 初始化配置（从模板拷贝）：

```bash
mkdir -p ~/.clipal
cp examples/config.yaml ~/.clipal/config.yaml
cp examples/claude-code.yaml ~/.clipal/claude-code.yaml
cp examples/codex.yaml ~/.clipal/codex.yaml
cp examples/gemini.yaml ~/.clipal/gemini.yaml
```

4) 编辑 `~/.clipal/*.yaml`，填入你的 `api_key`（以及需要的话 `base_url`）。

5) 启动并检查健康：

```bash
clipal --log-level debug
curl -fsS http://127.0.0.1:3333/health
```

6) 配置你的客户端（Claude Code / Codex CLI / Gemini CLI），见下方“客户端配置”。

### 安装

**方式 A：下载预编译二进制（推荐）**

从 [Releases](https://github.com/lansespirit/Clipal/releases) 下载对应平台的二进制文件。

```bash
# macOS / Linux
chmod +x clipal
sudo mv clipal /usr/local/bin/

# Windows
# 将 clipal.exe 添加到 PATH
```

**方式 B：从源码构建（当没有 Release 或你想自己编译时）**

```bash
git clone https://github.com/lansespirit/Clipal.git
cd Clipal
go build -o clipal ./cmd/clipal
sudo mv clipal /usr/local/bin/
```

### 运行

```bash
# 使用默认配置目录 (~/.clipal/)
clipal

# 指定配置目录
clipal --config-dir /path/to/config

# 指定监听地址（覆盖配置文件）
clipal --listen-addr 0.0.0.0

# 指定端口（覆盖配置文件）
clipal --port 8080

# 指定日志级别（覆盖配置文件）
clipal --log-level debug
```

### 更新

`clipal update` 会从 `lansespirit/Clipal` 下载最新 Release 的二进制，使用 `checksums.txt` 做 SHA256 校验，并替换当前可执行文件。

```bash
clipal update
clipal update --check
clipal update --dry-run
```

### 后台运行

```bash
# Linux/macOS - 临时后台运行（推荐配合内置落盘日志）
# 先在 ~/.clipal/config.yaml 设置 log_stdout: false，然后：
nohup clipal >/dev/null 2>&1 &
```

长期后台运行与开机自启（systemd / launchd / 任务计划程序）请看：

- [macOS](docs/zh/macos.md)
- [Linux](docs/zh/linux.md)
- [Windows](docs/zh/windows.md)

### 客户端配置

#### Claude Code

编辑 `~/.claude/settings.json`:

```json
{
  "env": {
    "ANTHROPIC_AUTH_TOKEN": "any-value",
    "ANTHROPIC_BASE_URL": "http://127.0.0.1:3333/claudecode"
  }
}
```

#### Codex CLI

编辑 `~/.codex/config.toml`:

```toml
model_provider = "clipal"

[model_providers.clipal]
name = "clipal"
base_url = "http://localhost:3333/codex"
```

#### Gemini CLI

```bash
export GEMINI_API_BASE="http://localhost:3333/gemini"
```

## 日志输出

clipal 默认同时输出到 stdout 与日志目录（按天滚动，默认保留 7 天）。

- 默认日志目录：`<config-dir>/logs`（例如 `~/.clipal/logs`）
- 日志文件：`clipal-YYYY-MM-DD.log`

后台静默运行建议在 `~/.clipal/config.yaml` 设置：

```yaml
log_stdout: false
log_retention_days: 7
# log_dir: ""  # 留空则默认 ~/.clipal/logs
```

```
[INFO]  2024-01-01 12:00:00 clipal starting on :3333
[INFO]  2024-01-01 12:00:00 loaded 3 providers for claude-code
[INFO]  2024-01-01 12:00:00 loaded 2 providers for codex
[INFO]  2024-01-01 12:00:00 loaded 2 providers for gemini
[DEBUG] 2024-01-01 12:00:05 [claudecode] request received: POST /v1/messages
[DEBUG] 2024-01-01 12:00:05 [claudecode] forwarding to: anthropic-direct
[WARN]  2024-01-01 12:00:06 [claudecode] anthropic-direct failed: 503, switching to openrouter
[INFO]  2024-01-01 12:00:07 [claudecode] request completed via openrouter
```

## 项目结构

```
clipal/
├── cmd/
│   └── clipal/
│       └── main.go           # 程序入口
├── internal/
│   ├── config/
│   │   └── config.go         # YAML 配置加载
│   ├── proxy/
│   │   ├── proxy.go          # 路由与代理请求构建
│   │   └── failover.go       # 故障转移与 Provider 降级/禁用
│   └── logger/
│       └── logger.go         # 日志模块
├── build/                    # 构建产物目录
│   ├── darwin-amd64/
│   │   └── clipal
│   ├── darwin-arm64/
│   │   └── clipal
│   ├── linux-amd64/
│   │   └── clipal
│   ├── linux-arm64/
│   │   └── clipal
│   └── windows-amd64/
│       └── clipal.exe
├── scripts/
│   └── build.sh              # 跨平台编译脚本
├── go.mod
├── go.sum
└── README.md
```

## 设计决策

### 为什么统一端口 + 路径前缀？

- **减少端口占用**：只需一个端口即可服务所有 CLI 工具
- **易于记忆**：无需记忆端口与 CLI 的对应关系
- **便于管理**：防火墙规则、反向代理配置更简单
- **统一入口**：所有请求通过同一入口，便于监控和调试

### 为什么不转换消息格式？

现代 API 供应商（如 OpenRouter、Together AI 等）已经提供了良好的格式兼容层。clipal 选择信任上游供应商的兼容能力，避免引入额外的复杂性和潜在的转换错误。

### 为什么使用 YAML 而非 JSON？

- YAML 支持注释，便于配置说明
- 更易于人工编辑
- 层级结构更清晰

### 为什么分三个配置文件？

不同的 CLI 工具可能需要不同的 API 供应商策略。例如：
- Claude Code 可能优先使用 Anthropic 官方 API
- Codex CLI 可能优先使用 OpenAI 兼容的供应商
- Gemini CLI 可能优先使用 Google 官方 API

分离配置让每个工具都能独立管理自己的供应商优先级。

### 为什么不记录历史和 Token 消耗？

保持极简。如需统计功能，建议：
- 使用 API 供应商的控制台查看用量
- 部署独立的监控服务
- 使用日志分析工具处理 clipal 的输出日志

## 与 ccNexus 的区别

| 特性 | clipal | ccNexus |
|------|--------|---------|
| 配置方式 | YAML 文件 | SQLite + Web UI |
| 消息格式转换 | 不支持 | 支持多种格式互转 |
| 统计功能 | 无 | 请求数、Token 用量等 |
| UI | 无 | Web UI + 桌面应用 |
| 目标用户 | CLI 工具用户 | 全面功能需求用户 |
| 复杂度 | 极简 | 功能丰富 |

## 开发

```bash
# 克隆项目
git clone https://github.com/lansespirit/clipal.git
cd clipal

# 构建当前平台
go build -o clipal ./cmd/clipal

# 运行测试
go test ./...

# 说明：Go 会在 GOCACHE 里写入编译缓存以加速后续构建/测试。
# 在某些受限环境（例如沙箱）中，默认缓存目录可能不可写而导致 go test 失败。
# 推荐用临时目录作为 GOCACHE，并在结束后清理：
tmp="$(mktemp -d "${TMPDIR:-/tmp}/go-build-cache.XXXXXX)"
GOCACHE="$tmp" go test ./...
rm -rf "$tmp"

# 交叉编译所有平台
./scripts/build.sh

# 或手动交叉编译
# macOS (Intel)
GOOS=darwin GOARCH=amd64 go build -o build/darwin-amd64/clipal ./cmd/clipal

# macOS (Apple Silicon)
GOOS=darwin GOARCH=arm64 go build -o build/darwin-arm64/clipal ./cmd/clipal

# Linux (x86_64)
GOOS=linux GOARCH=amd64 go build -o build/linux-amd64/clipal ./cmd/clipal

# Linux (ARM64)
GOOS=linux GOARCH=arm64 go build -o build/linux-arm64/clipal ./cmd/clipal

# Windows (x86_64)
GOOS=windows GOARCH=amd64 go build -o build/windows-amd64/clipal.exe ./cmd/clipal
```

### 支持的平台

| 平台 | 架构 | 文件名 |
|------|------|--------|
| macOS | Intel (amd64) | `clipal-darwin-amd64` |
| macOS | Apple Silicon (arm64) | `clipal-darwin-arm64` |
| Linux | x86_64 (amd64) | `clipal-linux-amd64` |
| Linux | ARM64 | `clipal-linux-arm64` |
| Windows | x86_64 (amd64) | `clipal-windows-amd64.exe` |

## License

MIT
