# Clipal Unified Ingress Plan

这组文档描述 Clipal 的修正后方案：

- 统一 `/clipal` ingress
- 统一协议识别链路
- 继续保留 Claude / Codex / Gemini 三组后端 provider 配置
- 恢复与用户心智一致的 Web UI 和状态展示

当前已经确认的核心结论：

- 统一的是入口，不是持久化 provider 模型
- `claude-code.yaml`、`codex.yaml`、`gemini.yaml` 继续作为路由真相
- `/claudecode`、`/codex`、`/gemini` 兼容入口继续保留
- compatibility alias 不能被过窄白名单误伤
- runtime state 可以按 capability 细化，但不能跨 backend pool 污染
- Web UI 不保留 unified provider 混排展示
- 后续需要为 auth-style 差异保留扩展缝

建议阅读顺序：

1. [01-target-architecture.md](01-target-architecture.md)
2. [02-config-and-data-model.md](02-config-and-data-model.md)
3. [03-routing-probing-and-runtime.md](03-routing-probing-and-runtime.md)
4. [04-implementation-phases.md](04-implementation-phases.md)
5. [05-risks-and-acceptance.md](05-risks-and-acceptance.md)
