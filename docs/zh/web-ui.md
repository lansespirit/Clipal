# Web UI 使用说明

## 访问方式

启动 Clipal 后，在浏览器打开：

```text
http://127.0.0.1:3333/
```

如果你修改了端口，请把地址中的 `3333` 换成你的实际端口。

## Web UI 能做什么

### Providers

- 按客户端分组查看 provider
- 新增、编辑、删除 provider
- 启用或禁用 provider
- 调整 `auto` / `manual` 模式
- 在 `manual` 模式下固定某个 provider
- 录入单个 `api_key` 或多行 `api_keys`

### Global Settings

- 修改监听地址和端口
- 修改日志级别
- 配置自动恢复、超时、请求体限制
- 配置熔断器
- 配置日志目录、保留天数、stdout 输出
- 配置桌面通知

### System Status

- 查看版本和运行时长
- 查看配置目录
- 查看各客户端当前模式、固定 provider、当前优先 provider
- 查看最近切换事件和最近请求结果
- 查看每个 provider 的运行态、已配置 key 数、可用 key 数

### Services

- 调用 `clipal service` 的常见能力
- 安装、启动、停止、重启、卸载后台服务
- 查看后台服务状态

### CLI Takeover

- 在 UI 中查看当前支持的一键接管项
- 一键接管 `Claude Code`、`Codex CLI`、`OpenCode`、`Gemini CLI`、`Continue`、`Aider` 和 `Goose` 的用户级配置
- 首次接管前自动创建备份，便于后续回滚
- 一键回滚到接管前捕获的原始文件状态
- 展示 Clipal 实际管理的目标配置文件路径
- 在 apply 前预览“当前文件”和“Apply 后将写入的结果”

说明：

- 当前阶段只修改用户级配置
- `Claude Code` 的一键接管只会更新 `ANTHROPIC_BASE_URL`，不会覆盖 `ANTHROPIC_AUTH_TOKEN`
- `OpenCode` 的一键接管会新增或更新 `provider.clipal`，并把当前 `model` 改写为 `clipal/<当前模型ID>`
- `Gemini CLI` 的一键接管只会更新 `~/.gemini/.env` 里的 `GEMINI_API_BASE`
- `Continue` 的一键接管会新增或更新用户级 `Clipal` 模型项
- `Aider` 的一键接管会更新 home 级 `openai-api-base`，并补一个最小 OpenAI 兼容 `model`
- `Goose` 的一键接管会管理 `~/.config/goose/custom_providers/` 下的独立 custom provider 文件
- 项目级、受管控或企业策略下发的配置仍可能覆盖最终生效结果
- 对已经由 Clipal 接管的配置重复执行 apply 会尽量保持幂等，不覆盖最初备份
- Apply 或 Rollback 后，建议重启客户端或新开一个会话，让它重新加载用户级配置

### Export

- 导出当前配置为 JSON，便于备份或迁移

## 状态页里常见的 provider 状态

- `disabled`：配置里手动禁用了
- `deactivated`：因为鉴权、额度或冷却逻辑被临时跳过
- `circuit_open`：熔断器处于打开状态
- `keys_exhausted`：该 provider 当前没有可用 key

## 安全边界

- Web UI 只允许本机访问
- 即使代理监听在 `0.0.0.0` 或 `::`，管理界面也会拒绝非 loopback 请求
- 管理 API 设计为本机使用，不提供独立认证层
- 变更类 API 请求要求 `X-Clipal-UI: 1`
- 带请求体的变更类 API 需要 `Content-Type: application/json`
- UI 不会直接展示每个 API Key 的明文

## 哪些修改通常立即生效

大多数配置变更会被热加载，无需重启，例如：

- provider 列表
- 优先级
- `mode`
- 失败切换相关参数
- 请求体限制
- 通知配置

通常需要重启才能完全生效的项目：

- 监听地址
- 监听端口
- 某些日志输出目标相关设置

## 相关文档

- [配置参考](config-reference.md)
- [路由与故障切换](routing-and-failover.md)
- [后台服务、状态与更新](services.md)
