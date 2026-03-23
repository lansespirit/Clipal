# 客户端接入

## 先理解一件事

Clipal 不是按“某个具体 App 名字”路由，而是按请求风格识别，且统一推荐通过 `/clipal` 接入：

- Claude 风格
- OpenAI / Codex 风格
- Gemini 风格

客户端 Base URL 建议统一指向 `/clipal`，再由 Clipal 根据请求路径识别协议族。旧的 `/claudecode`、`/codex`、`/gemini` 前缀仍保留为兼容别名。

需要注意一个路由细节：在 `/clipal` 下，通用的 `/v1/*` 资源路径默认按 OpenAI 兼容协议处理。Gemini 特有路由保留在 `/v1beta/*`、`/upload/*` 以及 `/v1beta/models/{model}:generateContent` 这类模型方法路径上。

## Claude Code

编辑 `~/.claude/settings.json`：

```json
{
  "env": {
    "ANTHROPIC_AUTH_TOKEN": "any-value",
    "ANTHROPIC_BASE_URL": "http://127.0.0.1:3333/clipal"
  }
}
```

说明：

- `ANTHROPIC_AUTH_TOKEN` 通常可填任意非空占位值
- 真正发往上游的认证信息由 Clipal 根据本地 provider 配置覆盖

## Codex CLI

编辑 `~/.codex/config.toml`：

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

## 通用 OpenAI 兼容客户端

对于支持“自定义 OpenAI Base URL”或“自定义 API Host”的本地客户端，通常使用：

```text
Base URL: http://127.0.0.1:3333/clipal
```

常见例子：

- Cherry Studio
- Kelivo
- Chatbox
- ChatWise
- 其他支持 OpenAI 兼容接口的桌面客户端

常见设置建议：

- Provider 类型：OpenAI Compatible / OpenAI API
- Base URL：`http://127.0.0.1:3333/clipal`
- API Key：若客户端强制要求，可填写任意非空占位值

注意：

- 客户端是否可用，取决于它发送的接口路径、请求体格式和模型参数是否与上游兼容
- 如果你在迁移旧配置，仍可继续使用兼容别名：`/claudecode`、`/codex`、`/gemini`

## 常见检查项

- Clipal 已启动：`clipal status`
- 健康检查正常：`curl -fsS http://127.0.0.1:3333/health`
- 客户端 Base URL 指向了 `http://127.0.0.1:3333/clipal`
- 本地客户端没有把旧的官方 Base URL 缓存在别处

如果接入后仍失败，继续看 [排障与 FAQ](troubleshooting.md)。
