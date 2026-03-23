# Clipal

![Clipal](assets/clipal-hero.jpg)

English: [README.md](README.md) | 中文: [README.zh-CN.md](README.zh-CN.md)

Clipal 是一个本地 LLM API 反向代理与管理工具。

它把多个上游 provider 统一收口到本机地址，支持自动切换、热重载、Web UI、后台服务和多 Key 管理。

除了 Claude Code、Codex CLI、Gemini CLI，也适合接入支持自定义 Base URL 的本地客户端，例如 Cherry Studio、Kelivo、Chatbox、ChatWise 等。

## 你可以用它做什么

- 给同一类客户端配置多个上游 provider，并按优先级自动切换
- 为不同客户端或协议分组维护独立配置
- 在 Web UI 中增删 provider、切换模式、查看状态、管理后台服务
- 在一个 provider 下配置多个 API Key，先在同 provider 内切换 key，再决定是否切 provider
- 通过 `clipal status`、`clipal service`、`clipal update` 管理本地运行状态
- 以单文件二进制运行在 macOS、Linux、Windows 上

## Web UI

![Clipal Web UI](assets/webUI.png)

## 适合哪些客户端

Clipal 现在将客户端入口统一规范到单一路由：

| 本地入口 | 典型用途 |
|----------|----------|
| `http://127.0.0.1:3333/clipal` | 首选统一入口，接收 Claude / OpenAI / Gemini 风格请求 |
| `http://127.0.0.1:3333/claudecode` | 兼容保留的 Claude 别名入口 |
| `http://127.0.0.1:3333/codex` | 兼容保留的 OpenAI 别名入口 |
| `http://127.0.0.1:3333/gemini` | 兼容保留的 Gemini 别名入口 |

新接入建议优先使用 `/clipal`。旧别名仍然保留，用于兼容现有配置和渐进迁移。是否完全可用，取决于客户端请求格式和上游 provider 的兼容程度。常见接入方式见 [docs/zh/client-setup.md](docs/zh/client-setup.md)。

## 快速开始

1. 从 [Releases](https://github.com/lansespirit/Clipal/releases) 下载对应平台的二进制。
   当前稳定版：[`v0.9.0`](https://github.com/lansespirit/Clipal/releases/tag/v0.9.0)
2. 放到 `PATH` 中并确认版本：

```bash
chmod +x clipal*
./clipal* --version
```

3. 初始化配置：

```bash
mkdir -p ~/.clipal
cp examples/config.yaml ~/.clipal/config.yaml
cp examples/claude.yaml ~/.clipal/claude.yaml
cp examples/openai.yaml ~/.clipal/openai.yaml
cp examples/gemini.yaml ~/.clipal/gemini.yaml
```

4. 编辑 `~/.clipal/*.yaml`，填入你的 `api_key` 或 `api_keys`。
5. 启动：

```bash
clipal
```

6. 验证并打开管理界面：

```bash
curl -fsS http://127.0.0.1:3333/health
clipal status
```

打开浏览器访问 `http://127.0.0.1:3333/`。

## 示例配置文件

- [examples/config.yaml](examples/config.yaml)
- [examples/claude.yaml](examples/claude.yaml)
- [examples/openai.yaml](examples/openai.yaml)
- [examples/gemini.yaml](examples/gemini.yaml)

## 常用命令

```bash
# 前台运行
clipal

# 查看状态
clipal status
clipal status --json

# 管理后台服务
clipal service install
clipal service status
clipal service restart

# 检查或更新版本
clipal update --check
clipal update
```

## 文档导航

- [快速开始](docs/zh/getting-started.md)
- [客户端接入](docs/zh/client-setup.md)
- [配置参考](docs/zh/config-reference.md)
- [Web UI 使用说明](docs/zh/web-ui.md)
- [路由与故障切换](docs/zh/routing-and-failover.md)
- [后台服务、状态与更新](docs/zh/services.md)
- [排障与 FAQ](docs/zh/troubleshooting.md)
- [macOS](docs/zh/macos.md) / [Linux](docs/zh/linux.md) / [Windows](docs/zh/windows.md)
- [文档首页](docs/zh/README.md)
- [Release Notes](release-notes/)

## 配置目录

默认配置目录：

- macOS / Linux: `~/.clipal/`
- Windows: `%USERPROFILE%\\.clipal\\`

默认文件：

```text
~/.clipal/
├── config.yaml
├── claude.yaml
├── openai.yaml
└── gemini.yaml
```

字段说明、示例和行为细节请看 [docs/zh/config-reference.md](docs/zh/config-reference.md)。

## 安全说明

- 代理默认监听 `127.0.0.1:3333`
- Web UI 只允许本机访问，即使代理监听在 `0.0.0.0` 或 `::`，管理界面仍会拒绝非本机请求
- Clipal 会根据本地配置覆盖上游认证头，客户端侧的占位 API Key 通常不会直接用于上游认证

## License

MIT
