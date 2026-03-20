# Clipal Unified Routing Plan

这组文档用于落地 Clipal 从当前 `codex` / `claudecode` / `gemini` 三分组代理，
演进到“统一入口 + 协议感知 + 自动探测 + 同协议路由”的方案。

当前已确认的核心结论：

- 统一入口使用 `/clipal`
- 兼容保留 `/codex`、`/claudecode`、`/gemini`
- Web UI 继续使用 `/`
- 管理 API 继续使用 `/api/*`
- 用户侧 provider 配置保持简单，只声明 `base_url`、`api_keys`、`protocols`
- 不引入用户可见的 `variants`
- 不引入模型 alias、模型映射、模型模式匹配
- 路由主依据是“入站协议 + provider 声明支持该协议 + 运行态可用性”
- 程序内部自动探测 endpoint、鉴权方式、模型列表并缓存
- failover 与健康状态从“按 client group”重构为“按 provider + protocol”

建议阅读顺序：

1. [01-target-architecture.md](01-target-architecture.md)
2. [02-config-and-data-model.md](02-config-and-data-model.md)
3. [03-routing-probing-and-runtime.md](03-routing-probing-and-runtime.md)
4. [04-implementation-phases.md](04-implementation-phases.md)
5. [05-risks-and-acceptance.md](05-risks-and-acceptance.md)
