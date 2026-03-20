# 01. Target Architecture

## 目标

将 Clipal 重构为一个：

- 单一统一入口的本地代理
- 能自动识别请求协议
- 能在同一份 provider 列表中做路由与故障切换
- 能自动探测 provider 对不同协议的真实可用能力
- 对用户保持低心智负担的系统

## 非目标

以下内容明确不纳入本轮方案：

- 不做模型 alias
- 不做模型映射
- 不做用户可见的 provider variants
- 不把多个 `base_url` 暴露给用户配置
- 不默认做跨协议模型路由
- 不为了统一入口移除现有兼容入口

## 最终入口布局

### 统一代理入口

- `/clipal`

示例：

- `/clipal/v1/messages`
- `/clipal/v1/messages/count_tokens`
- `/clipal/v1/chat/completions`
- `/clipal/v1/responses`
- `/clipal/v1beta/models/{model}:generateContent`

### 兼容入口

继续保留：

- `/claudecode`
- `/codex`
- `/gemini`

这些入口只作为别名存在，内部统一走同一条代理链路。

### 保留不变的入口

- Web UI: `/`
- Static: `/static/*`
- 管理 API: `/api/*`
- 健康检查: `/health`

## 核心架构分层

### 1. Ingress Layer

职责：

- 接收 `/clipal/*` 与旧别名入口请求
- 去除入口前缀
- 识别入站协议
- 构造统一内部请求上下文

这里不做 provider 选择，不做配置判断。

### 2. Protocol Detection Layer

按路径识别 `IngressProtocol`：

- `/v1/messages` -> `claude`
- `/v1/messages/count_tokens` -> `claude_count_tokens`
- `/v1/chat/completions` -> `openai_chat`
- `/v1/responses` -> `openai_responses`
- `/v1beta/models/...:generateContent` -> `gemini_generate`
- `/v1beta/models/...:streamGenerateContent` -> `gemini_stream_generate`

协议识别优先于模型名判断。

### 3. Routing Layer

路由仅依赖：

- 入站协议
- provider 是否声明支持该协议
- provider 当前健康状态
- provider 当前 key 可用性
- manual pin / priority / circuit breaker 状态

默认不以模型名作为主路由依据。

### 4. Adapter Layer

每种协议有一套内部适配器：

- Claude adapter
- OpenAI Chat adapter
- OpenAI Responses adapter
- Gemini adapter

职责：

- 构造上游请求路径
- 注入认证
- 判断 stream 完成标记
- 规范化错误识别
- 提供 protocol probe 能力

### 5. Capability Cache Layer

程序自动维护 provider 对各协议的探测结果，包括：

- 是否可用
- 实际请求路径
- 鉴权方式
- 模型列表
- 上次验证时间
- 失败原因

该层是内部实现细节，不作为用户主配置。

### 6. Runtime State Layer

所有运行态改为按 `provider + protocol` 维护：

- current key index
- deactivated state
- circuit breaker
- last request
- last success
- last failure

不再按旧的 `codex` / `claudecode` / `gemini` client group 维护主状态。

## 设计原则

### 原则 1：统一内核，兼容外部

内部只保留一套路由内核，外部同时支持：

- 新统一入口 `/clipal`
- 旧兼容入口 `/codex`、`/claudecode`、`/gemini`

### 原则 2：协议优先于模型

模型名由客户端决定并透传。

服务端主要负责：

- 判定请求属于哪种协议
- 找到支持该协议的 provider

### 原则 3：用户配置简单，程序内部复杂

用户只需要理解：

- 这个 provider 是什么
- 它的 `base_url`
- 它的 `api_keys`
- 它支持哪些协议

用户不需要理解：

- 协议变体
- 内部 endpoint 推导
- 探测缓存
- 真实上游请求路径

### 原则 4：自动探测优于手工填写细节

对于常见 provider，程序应自动：

- 检查各协议是否可达
- 推断鉴权方式
- 获取模型列表
- 缓存结果

### 原则 5：不为“统一”牺牲可调试性

日志、状态页、错误信息都应明确体现：

- ingress protocol
- selected provider
- selected protocol
- chosen key
- probe/capability status

## 目标收益

- 降低用户理解成本
- 减少三分组配置重复
- 统一 failover 内核
- 为未来新增协议留出扩展点
- 保持与现有客户端接入方式兼容
