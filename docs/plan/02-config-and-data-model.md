# 02. Config And Data Model

## 配置设计目标

配置模型需要同时满足两件事：

- 支持统一 ingress 与协议识别
- 保持后端 provider 池分离

因此本轮配置目标是：

- 保留三份 provider 配置文件
- 保留每组独立的 priority / enabled / mode / pinned_provider 语义
- 不把不同协议族的 key 池自动合并
- 允许内部构造统一请求上下文，但不改变持久化真相

## 用户配置结构

### 全局配置

`config.yaml`

负责：

- 监听地址
- 端口
- 日志
- failover / circuit breaker 全局参数

### Claude 配置

`claude-code.yaml`

```yaml
mode: auto
pinned_provider: ""

providers:
  - name: "anthropic-direct"
    base_url: "https://api.anthropic.com"
    api_key: "sk-ant-xxx"
    priority: 1
    enabled: true
```

### Codex / OpenAI 配置

`codex.yaml`

```yaml
mode: auto
pinned_provider: ""

providers:
  - name: "openrouter"
    base_url: "https://openrouter.ai/api"
    api_keys:
      - "sk-or-xxx"
      - "sk-or-yyy"
    priority: 1
    enabled: true
```

### Gemini 配置

`gemini.yaml`

```yaml
mode: auto
pinned_provider: ""

providers:
  - name: "gemini-direct"
    base_url: "https://generativelanguage.googleapis.com"
    api_key: "AIza..."
    priority: 1
    enabled: true
```

## 为什么不做统一 provider 持久化

统一 provider 列表在概念上很干净，但在 Clipal 这个场景下会引入更大的实际问题：

- 不同协议族的权重体系会混淆
- 不同渠道的 key 池会被误合并
- legacy `mode` / `pinned_provider` 语义很难无损迁移
- UI 会把用户本来清晰的三组操作心智打乱

结论是：

- “统一 ingress” 值得做
- “统一 provider 持久化模型” 当前不值得做

## Go 结构体建议

继续围绕现有三组配置模型演进：

```go
type ClientMode string

const (
    ClientModeAuto   ClientMode = "auto"
    ClientModeManual ClientMode = "manual"
)

type Provider struct {
    Name     string   `yaml:"name"`
    BaseURL  string   `yaml:"base_url"`
    APIKey   string   `yaml:"api_key,omitempty"`
    APIKeys  []string `yaml:"api_keys,omitempty"`
    Priority int      `yaml:"priority"`
    Enabled  *bool    `yaml:"enabled,omitempty"`
}

type ClientConfig struct {
    Mode           ClientMode `yaml:"mode"`
    PinnedProvider string     `yaml:"pinned_provider"`
    Providers      []Provider `yaml:"providers"`
}

type Config struct {
    Global     GlobalConfig
    ClaudeCode ClientConfig
    Codex      ClientConfig
    Gemini     ClientConfig
}
```

## 内部数据模型

### RequestContext

统一入口需要一个内部请求上下文：

```go
type IngressProtocol string
type InternalCapability string

type RequestContext struct {
    IngressPrefix   string
    VisibleProtocol IngressProtocol
    Capability      InternalCapability
    UpstreamPath    string
}
```

这个上下文是运行时对象，不进入用户配置。

### Backend Pool Mapping

协议识别完成后，需要映射到后端池：

```go
Claude                -> ClaudeCode pool
OpenAI Chat           -> Codex pool
OpenAI Responses      -> Codex pool
Gemini Generate       -> Gemini pool
Gemini StreamGenerate -> Gemini pool
```

### Runtime State

运行态可以继续细化，但必须局限在各自 backend pool 内。

推荐方向：

```go
type ProviderCapabilityRuntime struct {
    CurrentKeyIndex int

    DeactivatedReason string
    DeactivatedUntil  time.Time

    CircuitState  string
    CircuitOpenIn time.Duration

    LastRequest *RequestOutcomeEvent
    LastSwitch  *ProviderSwitchEvent
}
```

其中 capability 可以是：

- `claude_messages`
- `claude_count_tokens`
- `openai_chat_completions`
- `openai_responses`
- `gemini_generate_content`
- `gemini_stream_generate_content`

这些 capability 用于请求识别、runtime 事件和日志标签，不等于都拥有独立的 current provider 指针。

但这些 capability 只用于 runtime 和 adapter，不用于用户配置分组。

## 配置校验规则

继续保留并强化以下规则：

- `mode` 只能是 `auto | manual`
- `manual` 下 `pinned_provider` 必须存在且启用
- `api_key` 与 `api_keys` 不能同时设置
- key 至少有一个
- `priority >= 1`
- provider 名称在同一配置文件内唯一

不新增统一 `protocols[]` 作为用户必填字段。

## 热重载规则

继续监听：

- `config.yaml`
- `claude-code.yaml`
- `codex.yaml`
- `gemini.yaml`

热重载要求：

- 保持监听地址和端口在运行时稳定
- 尽量保留各 backend pool 内的 runtime state
- 不跨 backend pool 继承 provider 状态

## 迁移策略

本轮不做“旧三份配置自动迁移为一份新配置”。

迁移策略改为：

- 保持现有三份配置继续作为主配置
- 新增 `/clipal` 入口不要求用户改配置结构
- 如后续仍想探索统一 provider 视图，只能作为只读投影或实验性导出，不得成为默认路由真相

## 不纳入主配置的内容

以下内容仍不建议进入用户主配置：

- 真实探测出来的 endpoint path
- models 列表
- auth style 推断结果
- stream 完成标记规则
- provider capability cache

这些内容如果将来需要，应放在内部缓存或状态页，不应干扰用户配置。
