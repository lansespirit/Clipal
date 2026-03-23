# 01. Target Architecture

## 目标

本轮重构的目标是把 Clipal 收敛为：

- 一个统一的 ingress 入口
- 一套统一的协议识别链路
- 三组继续分离的后端 provider 池
- 一套更清晰的 runtime / failover / adapter 边界
- 一个不增加用户心智负担的系统

这里的关键约束是：

- 统一的是入口和内部处理链路
- 不统一的是用户配置、权重体系、key 池和 Web UI 分组

## 非目标

以下内容明确不纳入本轮：

- 不把 `claude-code.yaml`、`codex.yaml`、`gemini.yaml` 合并成一份路由真相
- 不引入统一 `providers.yaml` 作为默认持久化配置
- 不把 Claude / OpenAI / Gemini 的 provider 权重混排到同一个用户视图
- 不移除现有兼容入口
- 不做模型 alias
- 不做模型映射
- 不做跨协议模型路由
- 不在这一轮做完整的 capability cache / probe 平台

## 最终入口布局

### 统一入口

- `/clipal`

示例：

- `/clipal/v1/messages`
- `/clipal/v1/messages/count_tokens`
- `/clipal/v1/chat/completions`
- `/clipal/v1/responses`
- `/clipal/v1beta/models/{model}:generateContent`
- `/clipal/v1beta/models/{model}:streamGenerateContent`

### 兼容入口

继续保留：

- `/claudecode`
- `/codex`
- `/gemini`

兼容入口的定位是：

- 继续服务现有客户端
- 进入统一处理函数
- 但兼容语义仍尽量保持宽松，不因 `/clipal` 的协议检测而被过度收紧

### 保留不变的入口

- Web UI: `/`
- Static: `/static/*`
- 管理 API: `/api/*`
- 健康检查: `/health`

## 核心架构分层

### 1. Ingress Layer

职责：

- 接收 `/clipal/*` 与兼容入口请求
- 去除入口前缀
- 识别或推断请求所属协议族
- 构造统一的内部请求上下文

这一层只负责“看懂请求”，不负责“决定用哪个 provider”。

### 2. Protocol Detection Layer

`/clipal` 入口需要显式识别协议族，例如：

- `/v1/messages` -> Claude
- `/v1/messages/count_tokens` -> Claude Count Tokens
- `/v1/chat/completions` -> OpenAI Chat
- `/v1/responses` -> OpenAI Responses
- `/v1/files` -> OpenAI Files
- `/v1beta/models/...:generateContent` -> Gemini Generate
- `/v1beta/models/...:streamGenerateContent` -> Gemini Stream Generate

兼容入口的要求不同：

- `/claudecode/*`、`/codex/*`、`/gemini/*` 继续以兼容性优先
- 不应因为白名单过窄而把旧客户端的合法子路径直接拦掉

### 3. Backend Routing Layer

路由仍然分三组后端池：

- Claude backend pool
- Codex/OpenAI backend pool
- Gemini backend pool

`/clipal` 的价值是统一入口和协议识别，不是把三组 provider 池合并成一个池。

### 4. Adapter Layer

按协议族组织 adapter：

- Claude adapter
- OpenAI Chat adapter
- OpenAI Responses adapter
- Gemini adapter

职责：

- 构造上游 URL
- 注入鉴权
- 判断 stream 完成标记
- 识别协议特有错误

adapter 服务于协议差异，不替代 backend pool 的边界。

### 5. Runtime State Layer

运行态可以细化，但必须建立在正确的 backend pool 边界上。

推荐粒度：

- Claude pool 内，按 operation / capability 细分
- Codex pool 内，区分 Chat 与 Responses
- Gemini pool 内，区分 Generate 与 Stream Generate

这意味着：

- 可以细化 runtime state
- 但不能把不同后端配置池的 key / weight / enabled 状态揉成一个用户模型

## 设计原则

### 原则 1：统一入口，不统一用户配置模型

用户仍按既有心智维护三组 provider：

- Claude
- Codex/OpenAI
- Gemini

这比“统一 provider 列表”更稳，也更符合权重与渠道管理方式。

### 原则 2：协议识别优先，配置隔离优先

请求可以统一识别，但配置真相必须保留隔离边界，避免：

- 权重体系混淆
- key 池跨协议误用
- 某组 provider 状态污染另一组

### 原则 3：兼容入口宽松，统一入口明确

`/clipal` 可以更明确地做协议识别。

兼容入口必须优先保证已有客户端可继续工作，不应做过窄的 exact-match 限制。

### 原则 4：UI 服从用户心智

Web UI 的职责是让用户更容易管理配置，而不是展示内部抽象。

如果统一展示不能提升用户理解，就不应该保留。

### 原则 5：为兼容性保留扩展缝，不一次性过度设计

尤其是：

- 协议与 auth style 的解耦
- 更多 OpenAI family endpoints 的放宽
- Gemini-compatible upstream 的差异处理

这些都需要预留边界，但不必在本轮一次性做成大平台。

## 目标收益

- `/clipal` 为未来客户端接入提供统一入口
- 旧客户端继续可用
- 后端 provider 权重和 key 体系不被打乱
- runtime / failover / adapter 边界更清晰
- Web UI 保持低心智负担
