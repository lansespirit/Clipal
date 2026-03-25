# Clipal

<div align="center">
  <img src="assets/Clipal-hancock5.jpeg" alt="Clipal" width="100%">
  <p><b>你的终极本地 LLM API 网关与管理器</b></p>
  <p>
    <a href="README.md">English</a> | <a href="README.zh-CN.md">中文</a>
  </p>
</div>

---

**Clipal** 是一款专为开发者生产力打造的本地 LLM API 反向代理与管理工具。如果你正在使用诸如 Claude Code、Continue、Aider 或 Cherry Studio 等 AI 工具，Clipal 将成为你的智能流量大管家。它将多个上游大模型服务统一收口，支持自动故障切换、API Key 轮询，并提供了一个美观的 Web UI——让你专注于写代码，而不是折腾配置文件。

## ✨ 为什么选择 Clipal？（核心优势）

### 🚀 **一键 CLI 接管 (CLI Takeover)**
告别手动寻找、修改隐藏配置文件的烦恼。在 Web UI 中只需一键，Clipal 就能自动接管 **Claude Code、Codex CLI、OpenCode、Gemini CLI、Continue、Aider** 以及 **Goose** 的配置。
- 自动帮你配置本地 Base URL。
- 接管前自动备份原始配置。
- 随时支持一键回滚。

### 🛡️ **坚如磐石的故障切换与多 Key 轮询**
遇到并发限制、速率限制或者余额耗尽导致生成中断？
- **多 Key 轮询**：为单个 provider 配置多个 API Key，Clipal 会在同 provider 内自动重试并轮换 Key，直至成功。
- **优先级自动容灾**：当主模型/服务商不可用时，基于预设优先级秒级无缝切换到备用模型，自带断路器和并发阻断机制。

### 🎛️ **美观且强大的本地 Web UI**
可视化管理你的 AI 工作流。在这里增删、停用 provider，或者在“手动模式”下置顶特定模型，亦或调整全局运行参数。所有更改**热重载**生效，无需重启服务。

![Clipal Web UI](assets/webUI.png)

### ⚡ **无感知的后台守护服务**
Clipal 编译为单文件二进制，跨平台支持 macOS、Linux 和 Windows。
只需敲入 `clipal service install` 和 `clipal service start`，它就会静默在后台永远为你跑着。想要查看状态或重启？用 `clipal status` 和 `clipal restart` 瞬间搞定。

---

## 🔌 广泛的客户端支持

Clipal 现已将所有客户端入口统一规范到单一路由：`http://127.0.0.1:3333/clipal`。
它原生支持智能识别和兼容以下请求风格：
- **Anthropic / Claude**
- **OpenAI / Codex**
- **Google Gemini**

**常见支持工具：**
- **AI 编程助手：** Claude Code、Codex CLI、OpenCode、Gemini CLI、Continue、Aider、Goose
- **桌面端 AI 聊天：** Cherry Studio、Kelivo、Chatbox、ChatWise (兼容 OpenAI API)

---

## ⚡ 快速开始

### 1. 下载安装
前往 [Releases](https://github.com/lansespirit/Clipal/releases) 页面下载对应系统的二进制文件（当前稳定版：[`v0.11.2`](https://github.com/lansespirit/Clipal/releases/tag/v0.11.2)），并放入环境变量 `PATH` 中。
```bash
chmod +x clipal*
./clipal* --version
```

### 2. 初始化配置
```bash
mkdir -p ~/.clipal
cp examples/config.yaml ~/.clipal/config.yaml
cp examples/claude.yaml ~/.clipal/claude.yaml
cp examples/openai.yaml ~/.clipal/openai.yaml
cp examples/gemini.yaml ~/.clipal/gemini.yaml
```
*根据需要编辑 `~/.clipal/*.yaml`，填入你的 API Key。*

### 3. 运行与管理
在前台直接启动：
```bash
clipal
```
或者将其安装为后台服务：
```bash
clipal service install
clipal service start
```

### 4. 访问管理后台
打开浏览器访问 `http://127.0.0.1:3333/`，即可管理所有模型并为你的常用 AI 工具开启 **CLI Takeover**。

---

## 📖 完整文档导航

深入了解 Clipal 的全部能力：
- 🚀 [快速开始](docs/zh/getting-started.md)
- 🔌 [客户端接入指南](docs/zh/client-setup.md)
- ⚙️ [配置参考](docs/zh/config-reference.md)
- 🖥️ [Web UI 使用说明](docs/zh/web-ui.md)
- 🔀 [路由与故障切换魔法](docs/zh/routing-and-failover.md)
- 🛠️ [后台服务、状态与更新](docs/zh/services.md)
- 📚 [文档首页](docs/zh/README.md) & [Release Notes](release-notes/)

## 🔒 隐私与安全

- **100% 本地运行**：默认仅监听 `127.0.0.1:3333`。
- **Web UI 隔离保护**：即使代理开启了对外网段访问 (`0.0.0.0`)，Web 管理界面也严格强制仅限本机 (loopback) 访问。
- **真 Key 替换**：你在 Clipal 中配置的上游 API Key 只存在本地，Clipal 会在代理时自动覆盖并注入到真正的请求中，你可以在客户端放心地填入任何占位符。

<div align="center">
  <img src="assets/Clipal-luffy2.jpeg" alt="Clipal" width="100%">
</div>

## 📄 License
MIT
