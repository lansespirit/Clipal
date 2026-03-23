# 04. Implementation Phases

## 目标

通过分阶段修正当前方案，达到：

- 保留 `/clipal` 统一入口
- 恢复分离的 backend provider 配置与心智
- 修复兼容入口回归风险
- 保持 Web UI 与状态展示简单可理解

## Phase 0: 文档重定向

### 目标

先把错误方向从计划文档里移除，避免继续沿“统一 provider 持久化/UI”推进。

### 工作项

- 重写 `docs/plan/*`
- 明确“统一 ingress，后端 provider 分离”
- 明确 Web UI 恢复三分组
- 明确兼容入口宽松保留

### 验收

- 所有计划文档不再把 unified provider 作为目标
- 后续实现计划与文档基线一致

## Phase 1: 统一 ingress，不改配置真相

### 目标

让 `/clipal` 与兼容入口进入同一条处理函数，但不改变三套 backend 配置模型。

### 工作项

- 保留 `/clipal`
- 保留 `/claudecode`、`/codex`、`/gemini`
- 建立 `RequestContext`
- `/clipal` 执行协议识别
- compatibility alias 保留宽松透传语义

### 主要改动文件

- `internal/proxy/proxy.go`
- `internal/proxy/protocols.go`
- `internal/proxy/failover_protocol.go`
- `internal/proxy/failover_stream.go`

### 验收

- `/clipal` 可识别 Claude / OpenAI / Gemini 请求
- 兼容入口行为不被过度收紧

## Phase 2: 恢复分离的配置与热重载

### 目标

重新确认 `claude-code.yaml`、`codex.yaml`、`gemini.yaml` 是路由真相。

### 工作项

- 恢复 `ClientConfig` 为主配置模型
- 恢复 mode / pinned_provider 语义
- 恢复按三份配置文件热重载
- 移除 unified `providers.yaml` 作为默认持久化目标

### 主要改动文件

- `internal/config/config.go`
- `internal/config/config_test.go`
- `internal/proxy/reload_state_test.go`

### 验收

- 三份配置文件继续作为运行时真实来源
- manual / pinned_provider 不被静默丢弃

## Phase 3: Runtime 与 adapter 边界修正

### 目标

保留 capability-aware runtime 的优点，但把它限制在各 backend pool 内。

### 工作项

- 重新绑定 runtime state 到正确 pool
- OpenAI Chat / Responses 共享 Codex pool
- Claude Count Tokens 保持特殊策略
- adapter 继续处理协议差异

### 主要改动文件

- `internal/proxy/proxy.go`
- `internal/proxy/failover_forward.go`
- `internal/proxy/status.go`
- `internal/proxy/adapters/*`

### 验收

- runtime state 不跨 backend pool 污染
- capability 细化不破坏原有权重和 key 池语义

## Phase 4: Web UI 与状态页回归到正确心智

### 目标

让用户界面与真实配置边界重新一致。

### 工作项

- Web UI 恢复三分组管理
- `/api/providers/<alias>` 恢复为主要 CRUD 接口
- 移除统一 provider 混排展示
- CLI status 按三组展示

### 主要改动文件

- `internal/web/api.go`
- `internal/web/config_store.go`
- `internal/web/types.go`
- `internal/web/static/app.js`
- `internal/web/static/index.html`
- `cmd/clipal/status.go`

### 验收

- 用户只看到 Claude / Codex / Gemini 三组
- 不再看到统一 provider 混排列表

## Phase 5: 兼容性补强与文档收尾

### 目标

把当前明确识别到的兼容性风险补到可接受程度。

### 工作项

- 放宽 compatibility alias 端点支持
- 为 auth-style 差异保留扩展缝
- 更新 README / config reference / routing 文档
- 补充迁移和风险说明

### 主要改动文件

- `internal/proxy/protocols.go`
- `internal/proxy/adapters/*`
- `docs/en/*`
- `docs/zh/*`
- `README.md`
- `README.zh-CN.md`

### 验收

- 旧客户端兼容性没有明显回归
- 文档与实际行为一致

## 推荐实施顺序

必须按以下顺序推进：

1. Phase 0
2. Phase 1
3. Phase 2
4. Phase 3
5. Phase 4
6. Phase 5

原因：

- 不先修正文档，后续实现会继续漂移
- 不先恢复配置真相，runtime 与 UI 无法稳定收敛
- 不先修正兼容入口，后续接入回归风险太高

## 当前明确放弃的旧路线

以下方向在本轮中止：

- unified `providers.yaml` 作为主配置
- unified provider 混排 Web UI
- 自动把三份配置合并为一个共享 key 池 / 权重池
- 以统一 provider 视角驱动 CLI status 和状态页
