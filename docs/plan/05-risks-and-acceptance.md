# 05. Risks And Acceptance

## 主要风险

### 风险 1：兼容入口被过度收紧

#### 描述

如果把 `/clipal` 的严格协议识别逻辑原样套到：

- `/claudecode/*`
- `/codex/*`
- `/gemini/*`

会导致已有客户端使用的合法子路径被 404 拦掉。

#### 缓解

- 区分 `/clipal` 与 compatibility alias 的处理策略
- 为 alias 增加回归测试
- 至少覆盖现有已知 OpenAI family endpoints

### 风险 2：配置边界再次漂移

#### 描述

如果实现时又把 runtime 或 Web 层偷偷依赖统一 provider 投影，会再次出现：

- 权重混淆
- key 池误合并
- UI 与真实配置不一致

#### 缓解

- 明确三份配置文件是唯一持久化真相
- unified projection 不进入默认实现
- code review 时重点检查 persistence path

### 风险 3：legacy mode / pinned_provider 语义丢失

#### 描述

如果恢复分离配置时没有明确处理：

- `mode`
- `pinned_provider`

老用户升级后会遇到静默行为变化。

#### 缓解

- 保持原语义
- 或明确报错，不允许静默降级
- 增加配置加载测试

### 风险 4：runtime state 虽细化了 capability，但越过 pool 边界

#### 描述

比如：

- OpenAI Responses 的失败影响了 Claude provider 选择
- Gemini 的 circuit breaker 影响了 Codex 路由

#### 缓解

- runtime state 只能在 backend pool 内细化
- reload state 继承按 pool 校验
- 增加 cross-pool isolation 测试

### 风险 5：协议与鉴权方式被错误绑定

#### 描述

如果 adapter 把协议直接等同于固定 auth style，会让部分兼容网关无法接入。

#### 缓解

- 当前实现可保留常见默认值
- 但必须预留 provider-level auth 扩展缝
- 文档中明确兼容性边界

### 风险 6：Web UI 继续输出无意义的统一视图

#### 描述

如果前端保留 unified provider 混排展示，用户会看到一个并不能帮助操作的界面，反而更难理解：

- 这条 provider 属于哪组
- 权重到底跟哪组比
- 为什么同名 provider 会出现在多个协议里

#### 缓解

- Web UI 恢复三分组
- 状态页按三组展示
- 不保留统一混排视图

## 测试策略

### 单元测试

至少覆盖：

- `/clipal` 协议识别
- compatibility alias 宽松透传
- 分离配置加载与校验
- manual / pinned_provider 保留
- reload state 在各 pool 内继承
- runtime state 隔离
- adapter auth 注入边界

### 集成测试

至少覆盖：

- `/clipal/v1/messages`
- `/clipal/v1/messages/count_tokens`
- `/clipal/v1/chat/completions`
- `/clipal/v1/responses`
- `/clipal/v1beta/models/...:generateContent`
- `/codex/*` 兼容入口
- `/claudecode/*` 兼容入口
- `/gemini/*` 兼容入口

### 手工验收场景

#### 场景 1：统一入口接 Claude

- 客户端指向 `/clipal`
- 发 `/v1/messages`
- 系统识别为 Claude
- 在 Claude pool 内完成路由

#### 场景 2：统一入口接 OpenAI Responses

- 客户端指向 `/clipal`
- 发 `/v1/responses`
- 系统识别为 OpenAI Responses
- 在 Codex pool 内完成路由

#### 场景 3：兼容入口不被误伤

- 现有 `/codex/*` 客户端配置不改
- 非最小白名单端点仍可继续工作

#### 场景 4：manual 模式保留

- 某组 backend 配置使用 `mode: manual`
- 升级后仍保持 pinned provider 语义

#### 场景 5：同组内 capability 隔离

- `openai_responses` 连续失败
- `openai_chat` 仍可工作
- 但它们都仍属于 Codex backend pool

#### 场景 6：UI 分组恢复

- Web UI 继续展示 Claude / Codex / Gemini 三组
- 用户无需理解统一 provider 视图

## 最终验收标准

当以下条件都满足时，本轮修正才算完成：

- `/clipal` 统一入口可稳定处理 Claude、OpenAI、Gemini 请求
- `/claudecode`、`/codex`、`/gemini` 继续可用
- `claude-code.yaml`、`codex.yaml`、`gemini.yaml` 继续是路由真相
- mode / pinned_provider 不被静默丢失
- 不同 backend pool 之间不共享 provider/key/weight 语义
- Web UI 恢复三分组管理和展示
- CLI status 恢复三分组展示
- 文档、配置、运行时行为完全一致
