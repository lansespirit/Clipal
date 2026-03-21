# 02. Config And Data Model

## 配置设计目标

新的配置模型应满足：

- 用户只维护一份 provider 列表
- 每个 provider 只有一个 `base_url`
- 每个 provider 可配置多 `api_keys`
- 每个 provider 明确声明支持哪些协议
- 不暴露内部变体概念
- 兼容当前的优先级、启用开关、manual pin、热重载

## 建议的用户配置结构

```yaml
mode: auto
pinned_provider: ""

providers:
  - name: "openrouter"
    base_url: "https://openrouter.ai/api"
    api_keys:
      - "sk-xxx"
      - "sk-yyy"
    protocols:
      - "openai_chat"
      - "openai_responses"
      - "claude"
    priority: 1
    enabled: true

  - name: "anthropic-direct"
    base_url: "https://api.anthropic.com"
    api_key: "sk-ant-xxx"
    protocols:
      - "claude"
      - "claude_count_tokens"
    priority: 2
    enabled: true

  - name: "gemini-direct"
    base_url: "https://generativelanguage.googleapis.com"
    api_key: "AIza..."
    protocols:
      - "gemini_generate"
      - "gemini_stream_generate"
    priority: 3
    enabled: true
```

## 配置字段说明

### 顶层

- `mode`: `auto | manual`
- `pinned_provider`: `manual` 模式下固定使用的 provider
- `providers[]`: 统一 provider 列表

### provider

- `name`: provider 名称，唯一
- `base_url`: 用户输入的唯一基础地址
- `api_key` / `api_keys`: 单 key 或多 key
- `protocols`: 用户声明支持的协议列表
- `priority`: 越小优先级越高
- `enabled`: 是否启用

## 协议枚举建议

第一版建议显式区分如下协议：

- `claude`
- `claude_count_tokens`
- `openai_chat`
- `openai_responses`
- `gemini_generate`
- `gemini_stream_generate`

说明：

- `claude_count_tokens` 单独列出，是为了兼容现有针对 `count_tokens` 的特殊策略
- `gemini_generate` 与 `gemini_stream_generate` 分开，有助于 probe 和错误诊断

## Go 结构体建议

```go
type ClientMode string

const (
    ClientModeAuto   ClientMode = "auto"
    ClientModeManual ClientMode = "manual"
)

type Protocol string

const (
    ProtocolClaude              Protocol = "claude"
    ProtocolClaudeCountTokens   Protocol = "claude_count_tokens"
    ProtocolOpenAIChat          Protocol = "openai_chat"
    ProtocolOpenAIResponses     Protocol = "openai_responses"
    ProtocolGeminiGenerate      Protocol = "gemini_generate"
    ProtocolGeminiStreamGenerate Protocol = "gemini_stream_generate"
)

type Provider struct {
    Name      string     `yaml:"name"`
    BaseURL   string     `yaml:"base_url"`
    APIKey    string     `yaml:"api_key,omitempty"`
    APIKeys   []string   `yaml:"api_keys,omitempty"`
    Protocols []Protocol `yaml:"protocols"`
    Priority  int        `yaml:"priority"`
    Enabled   *bool      `yaml:"enabled,omitempty"`
}

type RoutingConfig struct {
    Mode           ClientMode `yaml:"mode"`
    PinnedProvider string     `yaml:"pinned_provider"`
    Providers      []Provider `yaml:"providers"`
}
```

## 内部运行态模型

### Capability Cache

这部分不写入用户主配置，可单独持久化为内部缓存文件或运行时热缓存。

```go
type ProtocolCapability struct {
    Available      bool
    AuthStyle      string
    RequestPath    string
    ModelsPath     string
    Models         []string
    LastVerifiedAt time.Time
    LastError      string
}

type ProviderCapabilityCache struct {
    ProviderName string
    Protocols    map[Protocol]ProtocolCapability
}
```

### Runtime State

```go
type ProviderProtocolKey struct {
    Provider string
    Protocol Protocol
}

type ProviderProtocolRuntime struct {
    CurrentKeyIndex int

    DeactivatedReason string
    DeactivatedUntil  time.Time

    CircuitState  string
    CircuitOpenIn time.Duration

    LastRequest *RequestOutcomeEvent
    LastSwitch  *ProviderSwitchEvent
}
```

## 配置迁移方案

### 当前状态

当前项目是：

- `claude-code.yaml`
- `codex.yaml`
- `gemini.yaml`

三份独立配置。

### 迁移目标

迁移为一份统一路由配置，例如：

- `routing.yaml`

或保留现有文件命名习惯，但内部统一写入一份新文件。

### 推荐迁移策略

第一阶段不立刻删除旧文件，而是：

1. 启动时优先读取新统一配置
2. 若不存在新配置，则读取三份旧配置并自动合并
3. UI 仅编辑新统一配置
4. 保留一次性迁移导出能力，帮助用户确认

### 合并规则

- 同名 provider 若来自多个旧文件，视为同一 provider
- `protocols` 为各旧分组协议的并集
- `api_keys` 合并去重
- `priority` 取最小值
- 若 `base_url` 冲突，则保留为多个 provider，避免错误合并

## 验证规则

新的配置校验需要新增：

- `protocols` 不能为空
- `protocols` 内枚举值必须合法
- `manual` 模式下 `pinned_provider` 必须存在且启用
- `api_key` 与 `api_keys` 不能同时设置
- key 至少有一个

## 不建议纳入配置的内容

以下内容不建议加入用户主配置：

- 实际探测出的真实 endpoint path
- 实际 models 列表
- 鉴权推断结果
- stream 完成标记规则
- 供应商内部协议变体

这些都应由程序自动维护。
