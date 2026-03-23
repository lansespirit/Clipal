# 03. Routing, Probing, And Runtime

## 请求链路总览

本轮目标链路如下：

1. 接收请求
2. 判断入口前缀
3. 构造统一 `RequestContext`
4. 将请求映射到正确的 backend pool
5. 在该 pool 内执行 failover / key 轮转 / circuit breaker
6. 用对应 adapter 构造上游请求
7. 转发并返回结果
8. 更新该 pool 内的 runtime state

关键点：

- ingress 可以统一
- provider 选择不能跨 backend pool

## 入站协议识别

### `/clipal` 入口

`/clipal` 需要显式识别协议族与 capability，例如：

- `/clipal/v1/messages` -> Claude Messages
- `/clipal/v1/messages/count_tokens` -> Claude Count Tokens
- `/clipal/v1/chat/completions` -> OpenAI Chat
- `/clipal/v1/responses` -> OpenAI Responses
- `/clipal/v1/files` -> OpenAI Files
- `/clipal/v1beta/models/...:generateContent` -> Gemini Generate
- `/clipal/v1beta/models/...:streamGenerateContent` -> Gemini Stream Generate

### 兼容入口

兼容入口要求更保守：

- `/claudecode/*`
- `/codex/*`
- `/gemini/*`

这些入口进入统一处理函数，但不能只允许极小的白名单路径。

尤其是 `/codex/*`，需要为更多 OpenAI family endpoints 留出兼容空间，例如：

- models
- images
- audio
- 未来新增的 family endpoint

## 后端 pool 选择规则

协议识别后立即选择 backend pool：

- Claude 请求 -> `ClaudeCode` pool
- OpenAI Chat / Responses -> `Codex` pool
- Gemini 请求 -> `Gemini` pool

之后的所有逻辑都在该 pool 内完成：

- provider 选择
- key 轮转
- deactivation
- circuit breaker
- runtime snapshot

## 路由选择规则

在选定的 backend pool 内，沿用现有成熟规则：

1. 仅选择 `enabled` provider
2. manual 模式下仅允许 `pinned_provider`
3. 过滤当前 deactivated provider
4. 过滤没有可用 key 的 provider
5. 过滤 circuit open 的 provider
6. 按 priority 和现有 failover 状态选择

这部分不因 `/clipal` 入口而改变语义。

## Adapter 设计

仍按 capability 组织 adapter：

- Claude adapter
- OpenAI Chat adapter
- OpenAI Responses adapter
- Gemini adapter

### Adapter 负责

- URL 组装
- 鉴权注入
- 协议流式完成判定
- 协议错误分类

### Adapter 不负责

- 跨 pool 路由
- 统一 provider 视图
- 用户配置解释

## 鉴权兼容性边界

这一轮需要明确一个现实约束：

- 协议不等于 auth style

例如 Gemini 协议并不必然等于某一种固定鉴权方式。当前实现可以先保留最常见直连方式，但必须为后续兼容：

- provider-specific auth style
- protocol-compatible gateway

预留扩展缝。

## Runtime State 设计

### 设计目标

runtime state 要做到两件事：

- 不同 capability 之间尽量隔离
- 不突破 backend pool 边界

### 推荐粒度

在各 pool 内细化：

- Claude pool:
  - `claude_messages`
  - `claude_count_tokens` 仅作为 capability 标签，不维护独立 current provider
- Codex pool:
  - `openai_chat_completions`
  - `openai_responses`
- Gemini pool:
  - `gemini_generate_content`
  - `gemini_stream_generate_content`
  - `gemini_count_tokens` 仅作为 capability 标签，不维护独立 current provider

### 运行态字段

- current provider index
- current key index
- provider deactivation
- key deactivation
- circuit breaker state
- last request
- last switch

## Count Tokens 特殊处理

保留现有思想：

- `count_tokens` 是 advisory request
- 单次透传，不参与 failover / cooldown / breaker 状态变更

但它属于 Claude backend pool 内的一个 capability，而不是独立的用户配置组。

## Probe 与 Capability Cache

这部分本轮只保留轻量边界，不做重平台。

### 本轮要求

- adapter 代码不要把 auth style 写死成不可扩展结构
- runtime / status 代码为后续 capability 信息留接口
- 文档中承认后续需要更强兼容性

### 本轮不要求

- 完整 capability cache 文件格式
- 后台 probe 调度器
- 模型列表缓存
- 把 probe 结果纳入主路由依据

## 状态展示原则

状态页和 CLI status 继续按 backend group 展示：

- Claude
- Codex
- Gemini

可在组内展示更细 capability 状态，但不做统一 provider 混排视图。

这样更符合用户日常判断：

- Claude 这组现在怎么样
- Codex 这组现在怎么样
- Gemini 这组现在怎么样
