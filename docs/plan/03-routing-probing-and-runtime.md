# 03. Routing, Probing, And Runtime

## 路由链路总览

统一后的单次请求链路：

1. 接收请求
2. 去除入口前缀
3. 识别入站协议
4. 按协议筛选 provider
5. 读取 capability cache
6. 结合 failover / key / circuit breaker 选择 provider
7. 用对应协议 adapter 构造上游请求
8. 转发并流式返回
9. 记录运行态与统计

## 入站协议识别

### 统一入口 `/clipal`

示例映射：

- `/clipal/v1/messages` -> `claude`
- `/clipal/v1/messages/count_tokens` -> `claude_count_tokens`
- `/clipal/v1/chat/completions` -> `openai_chat`
- `/clipal/v1/responses` -> `openai_responses`
- `/clipal/v1beta/models/{model}:generateContent` -> `gemini_generate`
- `/clipal/v1beta/models/{model}:streamGenerateContent` -> `gemini_stream_generate`

### 旧别名入口

旧入口先去掉前缀，再按同样规则识别协议：

- `/claudecode/v1/messages`
- `/codex/v1/chat/completions`
- `/codex/v1/responses`
- `/gemini/v1beta/models/...`

## 路由选择规则

### 选择顺序

1. 仅选择 `enabled == true` 的 provider
2. 仅选择声明支持当前协议的 provider
3. 仅选择 capability cache 判定可用或尚未探测的 provider
4. 过滤掉当前 deactivated 的 provider + protocol
5. 过滤掉 circuit open 的 provider + protocol
6. 过滤掉当前没有可用 key 的 provider + protocol
7. 在 manual 模式下仅允许 `pinned_provider`
8. 否则按 priority 与最近成功记录选择

### 明确不做的事情

- 不按模型名前缀选 provider
- 不做模型 alias
- 不根据模型列表强制路由

模型名原样透传给最终上游。

## 协议 Adapter 设计

每种协议一套内部 adapter：

- `ClaudeAdapter`
- `OpenAIChatAdapter`
- `OpenAIResponsesAdapter`
- `GeminiAdapter`

### Adapter 负责

- 识别是否匹配当前协议
- 根据 `base_url` 和 probe 结果构建真实 URL
- 注入认证头或 query 参数
- 提供 probe 行为
- 提供 stream 完成判定
- 提供错误分类所需的协议特征

### Adapter 不负责

- provider 选择
- priority / failover 策略
- key 轮转

## Probe 设计

## 触发时机

- provider 新增后
- provider 编辑后
- 启动时首次加载后
- 周期性后台刷新
- 某协议首次请求但 capability cache 缺失时
- 某协议连续失败达到阈值后

## Probe 范围

只对用户声明支持的协议做探测。

### OpenAI Chat

可探测：

- `/v1/chat/completions`
- `/v1/models`
- Bearer 认证

### OpenAI Responses

可探测：

- `/v1/responses`
- `/v1/models`
- Bearer 认证

### Claude

可探测：

- `/v1/messages`
- 可选 `/v1/messages/count_tokens`
- `x-api-key` 与 Bearer 兼容性

### Gemini

可探测：

- `:generateContent`
- `:streamGenerateContent`
- query `key=...`

## Probe 结果缓存

建议将 probe 结果保存到独立缓存文件，例如：

- `runtime-capabilities.json`

理由：

- 不污染用户主配置
- 可热更新
- 容易清空重建
- 适合存放 models 列表和验证时间

## 模型列表缓存的用途

模型列表缓存只用于：

- UI 展示
- 连通性诊断
- 调试与 probe 可视化

不作为主路由决策条件。

## 运行态重构

## 当前问题

当前运行态主要按旧 client group 维护：

- codex
- claudecode
- gemini

这会导致同一 provider 在不同协议下的状态被混在一起。

## 新运行态粒度

运行态改为按：

- `provider + protocol`

维护。

例如：

- `openrouter + openai_chat`
- `openrouter + openai_responses`
- `openrouter + claude`

各自拥有独立状态。

## Failover

### Key 轮转

仍保持当前语义：

- 先在同 provider 内轮换 key
- key 全部不可用时，再切下一个 provider

但粒度改为当前协议下的 provider state。

### Circuit Breaker

circuit breaker 应绑定在 `provider + protocol` 上。

不要因为：

- `openrouter + openai_responses` 失败

就直接影响：

- `openrouter + openai_chat`

### Deactivation

deactivation 也绑定在 `provider + protocol + key` 上。

## Count Tokens 特殊处理

保留当前设计思想，但从“Claude client 特殊逻辑”改为“协议 + operation 特殊逻辑”：

- `claude_count_tokens` 请求可保持独立的 current provider / key pointer
- 可继续支持 `ignore_count_tokens_failover`

## 状态页重构建议

状态页应逐步从“按 client group 展示”过渡到：

- provider 列表
- 每个 provider 下展开协议能力
- 每个协议显示：
  - available
  - current key
  - key count
  - circuit state
  - deactivated reason
  - last request
  - last verified time

旧 client 视图可在过渡期保留。
