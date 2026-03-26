# Clipal

<div align="center">
  <img src="assets/Clipal-Hancock.jpeg" alt="Clipal" width="100%">
  <p><b>Your Ultimate Local LLM API Gateway & Manager</b></p>
  <p>
    <a href="README.md">English</a> | <a href="README.zh-CN.md">中文</a>
  </p>
</div>

---

**Clipal** is the local LLM API proxy built for developer productivity. If you use AI coding tools like Claude Code, Continue, Aider, or Cherry Studio, Clipal acts as your intelligent traffic controller. It consolidates multiple model providers, handles automatic failover, manages API keys, and offers a beautiful Web UI—so you can focus on coding, not configuring.

## Join the WeChat Group

Scan the QR code below to join the Clipal WeChat group and discuss usage, setup, and feedback with other users.

![Clipal WeChat Group QR Code](assets/wechat-group.png)

## ✨ Why Clipal? (Superpowers)

### 🚀 **One-Click CLI Takeover**
No more hunting for hidden config files. With a single click in the Web UI, Clipal can automatically take over the configurations for **Claude Code, Codex CLI, OpenCode, Gemini CLI, Continue, Aider**, and **Goose**. 
- It configures the local base URL for you.
- It backs up your original settings.
- It provides a safe rollback whenever you want.

### 🛡️ **Bulletproof Failover & Multi-Key Rotation**
Tired of hitting rate limits or empty balances mid-generation?
- **Multi-Key Pool**: Configure multiple `.api_keys` for a single provider. Clipal rotates them automatically and retries locally before giving up.
- **Priority Failover**: Fall back to secondary models or providers instantly with out-of-the-box circuit breaking and quota management.

### 🎛️ **Beautiful Local Web UI**
Manage your AI workflows visually. Add, edit, enable, or disable providers, pin a specific model, and manage global settings with a modern dashboard. All changes are hot-reloaded—no restarts required.

![Clipal Web UI](assets/webUI.png)

### ⚡ **Frictionless Background Service**
Clipal runs as a single, cross-platform binary on macOS, Linux, and Windows. 
Type `clipal service install` and `clipal service start` to keep it running silently in the background forever. Use `clipal status` or `clipal restart` for quick management.

---

## 🔌 Supported Clients

Clipal standardizes client ingress entirely on a single local route: `http://127.0.0.1:3333/clipal`.
It natively supports the request flavors of:
- **Anthropic / Claude**
- **OpenAI / Codex**
- **Google Gemini**

**Popular Supported Tools:**
- **AI Coding Assistants:** Claude Code, Codex CLI, OpenCode, Gemini CLI, Continue, Aider, Goose
- **Desktop Chat Clients:** Cherry Studio, Kelivo, Chatbox, ChatWise (via OpenAI compatibility)

---

## ⚡ Quick Start

### 1. Install Clipal
Download the standalone binary for your OS from [Releases](https://github.com/lansespirit/Clipal/releases) (Current stable release: [`v0.11.2`](https://github.com/lansespirit/Clipal/releases/tag/v0.11.2)) and put it on your `PATH`.
```bash
chmod +x clipal*
./clipal* --version
```

### 2. Initialize Configurations
```bash
mkdir -p ~/.clipal
cp examples/config.yaml ~/.clipal/config.yaml
cp examples/claude.yaml ~/.clipal/claude.yaml
cp examples/openai.yaml ~/.clipal/openai.yaml
cp examples/gemini.yaml ~/.clipal/gemini.yaml
```
*Edit the generated `~/.clipal/*.yaml` files to add your API keys.*

### 3. Run & Manage
Start Clipal in the foreground:
```bash
clipal
```
Or install it as a background service:
```bash
clipal service install
clipal service start
```

### 4. Open the Web UI
Visit `http://127.0.0.1:3333/` in your browser to manage providers and apply **CLI Takeover** for your favorite tools.

---

## 📖 Complete Documentation

Dive deeper into what Clipal can do:
- 🚀 [Getting Started](docs/en/getting-started.md)
- 🔌 [Client Setup Guide](docs/en/client-setup.md)
- ⚙️ [Config Reference](docs/en/config-reference.md)
- 🖥️ [Web UI Guide](docs/en/web-ui.md)
- 🔀 [Routing & Failover Magic](docs/en/routing-and-failover.md)
- 🛠️ [Services, Status, and Updates](docs/en/services.md)
- 📚 [Docs Home](docs/en/README.md) & [Release Notes](release-notes/)

## 🔒 Security & Privacy

- Clipal is fully local. The proxy listens on `127.0.0.1:3333` by default.
- The Web UI is strictly locked to localhost—even if the proxy listens externally, the management UI rejects non-loopback requests.
- Your upstream API keys are stored only on your machine and transparently injected into requests.

<div align="center">
  <img src="assets/Clipal-luffy3.jpeg" alt="Clipal" width="100%">
</div>

## 📄 License
MIT

## 🙏 Thanks
Thanks to the [linux.do](https://linux.do/) community for its support.
