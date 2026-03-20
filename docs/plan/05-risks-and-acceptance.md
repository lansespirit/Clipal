# 05. Risks And Acceptance

## 主要风险

## 风险 1：旧路由兼容回归

### 描述

当前已有用户直接依赖：

- `/codex`
- `/claudecode`
- `/gemini`

如果统一入口改造影响旧别名处理，现有客户端会失效。

### 缓解

- 旧入口长期保留
- 旧入口内部只做前缀去除，不保留独立逻辑分支
- 为旧入口增加回归测试

## 风险 2：配置迁移误合并

### 描述

旧三份配置可能存在：

- 同名 provider 不同 base_url
- 不同文件中相同 key 的重复或冲突

自动迁移可能错误合并。

### 缓解

- 合并规则对 `base_url` 冲突保守处理
- 提供迁移预览
- 导出迁移结果供用户确认

## 风险 3：probe 误判

### 描述

某些 provider：

- endpoint 行为不标准
- models API 权限不足
- 认证方式兼容但不完整

可能导致 probe 结果不稳定。

### 缓解

- 区分“未验证”“验证失败”“验证成功”
- models probe 失败不应直接判定主协议不可用
- 保留请求时按需惰性验证

## 风险 4：同一 provider 不同协议状态相互污染

### 描述

如果 runtime state 仍残留旧设计，很容易出现：

- OpenAI Responses 失败导致 Claude 也被判不可用

### 缓解

- 运行态键统一改为 `provider + protocol`
- 测试覆盖 cross-protocol isolation

## 风险 5：Web UI 心智切换过大

### 描述

现有 UI 是三分组视图，统一 provider 后展示方式变化较大。

### 缓解

- 第一版允许保留兼容视图
- 新视图强调“provider 支持哪些协议”
- 不暴露内部 variants 概念

## 风险 6：未知自定义 provider 的真实 endpoint 不可自动推断

### 描述

用户只给一个 `base_url` 时，若实际不同协议需要不同 host，程序无法无损推断。

### 缓解

- 常见 provider 用 preset 覆盖
- 未知 provider 使用单 base_url 假设
- 探测失败时给出明确提示
- 必要时允许用户拆成多个 provider

## 测试策略

## 单元测试

应新增或重构以下测试：

- 入站协议识别
- 旧别名入口到统一链路的前缀处理
- provider 合并与迁移
- `protocols[]` 校验
- capability cache 读写
- 按 `provider + protocol` 的 key 轮转与 circuit breaker

## 集成测试

至少覆盖：

- `/clipal/v1/messages`
- `/clipal/v1/chat/completions`
- `/clipal/v1/responses`
- `/clipal/...gemini...`
- `/codex/*` 旧兼容入口
- `/claudecode/*` 旧兼容入口
- `/gemini/*` 旧兼容入口

## 手工验收场景

### 场景 1：统一入口接 Claude

- 客户端指向 `/clipal`
- 发 `/v1/messages`
- 系统能正确识别为 `claude`
- 路由到支持 Claude 的 provider

### 场景 2：统一入口接 Codex Chat

- 客户端指向 `/clipal`
- 发 `/v1/chat/completions`
- 系统识别为 `openai_chat`
- 路由到支持 OpenAI Chat 的 provider

### 场景 3：统一入口接 Codex Responses

- 客户端指向 `/clipal`
- 发 `/v1/responses`
- 系统识别为 `openai_responses`
- 路由到支持该协议的 provider

### 场景 4：provider 多 key 轮转

- 同一 provider 下多个 key
- 一个 key 429 或 401
- 先切同 provider 的下一个 key
- 再在必要时切 provider

### 场景 5：同 provider 不同协议隔离

- `provider A + openai_responses` 连续失败
- `provider A + openai_chat` 仍可正常工作

### 场景 6：旧入口兼容

- 现有 `/codex`、`/claudecode`、`/gemini` 客户端配置不改
- 行为与迁移前保持一致

## 最终验收标准

当以下条件全部满足时，可认为该重构完成：

- `/clipal` 统一入口可稳定处理 Claude、OpenAI Chat、OpenAI Responses、Gemini 请求
- 旧入口仍可工作
- 用户侧 provider 配置不需要理解 variants
- 用户侧 provider 配置不需要填写多个 base_url
- 不引入模型 alias 和模型映射
- capability cache 能正确显示协议支持状态
- failover 粒度已切换为 `provider + protocol`
- Web UI 可管理统一 provider 与协议支持
- 文档、配置、运行时行为保持一致
