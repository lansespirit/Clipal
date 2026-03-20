# 04. Implementation Phases

## 目标

通过分阶段重构，避免一次性大改导致：

- 现有客户端接入失效
- Web UI 管理功能退化
- failover 逻辑回归
- 配置迁移不可控

## Phase 0: 文档与接口冻结

### 目标

冻结当前讨论结论，作为后续开发约束。

### 产出

- 本目录所有计划文档
- 统一入口命名：`/clipal`
- 非目标清单确认

## Phase 1: 统一代理内核，不改用户配置

### 目标

先统一内部请求处理链路，外部仍兼容旧结构。

### 工作项

- 引入 `IngressProtocol` 枚举
- 把旧 `/codex` `/claudecode` `/gemini` 三套路由收敛到同一处理函数
- 新增 `/clipal` 统一入口
- 把协议识别从 `ClientType` 迁移为基于 path 的 detection
- 保留现有配置文件不变

### 主要改动文件

- `internal/proxy/proxy.go`
- `internal/proxy/failover_protocol.go`
- `internal/proxy/failover_stream.go`

### 验收

- 旧入口行为不变
- `/clipal` 可正确代理同样的请求
- Web UI 不受影响

## Phase 2: 配置模型统一

### 目标

把三份 client config 重构成一份 unified routing config。

### 工作项

- 引入新的统一配置结构
- 保持旧配置兼容读取
- 在加载阶段做旧配置合并
- 新的校验逻辑支持 `protocols[]`
- 热重载改为监控新配置文件

### 主要改动文件

- `internal/config/config.go`
- `internal/web/yaml_format.go`
- `internal/web/api.go`

### 验收

- 新配置可正常加载
- 旧配置可自动兼容
- provider 列表去重与协议合并逻辑可工作

## Phase 3: Adapter 与 Probe 基础设施

### 目标

建立内部协议 adapter 与 capability cache。

### 工作项

- 定义 adapter 接口
- 实现 Claude/OpenAI Chat/OpenAI Responses/Gemini adapters
- 实现 probe runner
- 实现 capability cache 读写
- 请求转发从启发式 header 逻辑迁移到 adapter 注入

### 主要改动文件

- 新增 `internal/proxy/adapters/*`
- 新增 `internal/proxy/probe/*` 或同层模块
- 重构 `internal/proxy/proxy.go`

### 验收

- provider 新增后可自动完成协议探测
- capability cache 可展示探测结果
- 同协议请求可正确使用对应 adapter 发送

## Phase 4: Runtime State 粒度重构

### 目标

把 failover、deactivation、circuit breaker 从 client group 粒度改为 provider + protocol。

### 工作项

- 重构运行态 key
- 重构状态快照
- 重构 key 轮转索引
- 重构 last request / last switch 归属

### 主要改动文件

- `internal/proxy/status.go`
- `internal/proxy/failover_forward.go`
- `internal/proxy/failover_manual.go`
- `internal/proxy/circuit_breaker.go`

### 验收

- 同一 provider 的不同协议互不污染状态
- key 轮转与 circuit breaker 粒度正确

## Phase 5: Web UI 与状态页升级

### 目标

让 Web UI 从“三分组 provider 管理器”升级为“统一 provider + 协议能力视图”。

### 工作项

- provider 表单增加 `protocols[]`
- 状态页增加协议能力与 probe 展示
- 编辑页去掉旧三分组心智
- 导出与导入格式升级

### 主要改动文件

- `internal/web/types.go`
- `internal/web/api.go`
- `internal/web/static/app.js`
- `internal/web/static/index.html`

### 验收

- 可在 UI 中添加统一 provider
- 可直观看到各协议支持与探测情况
- 不要求用户理解 variants

## Phase 6: 文档与迁移收尾

### 目标

完成文档更新与兼容策略说明。

### 工作项

- 更新 README
- 更新 client setup
- 更新 config reference
- 新增 migration guide
- 说明 `/clipal` 新入口与旧别名并存策略

### 验收

- 文档与实际行为一致
- 新老用户都能完成迁移

## 推荐实施顺序

必须严格按顺序：

1. Phase 1
2. Phase 2
3. Phase 3
4. Phase 4
5. Phase 5
6. Phase 6

不建议跳过：

- Phase 1
- Phase 3
- Phase 4

因为这三阶段分别决定：

- 统一链路是否成立
- 协议能力抽象是否成立
- failover 粒度是否正确

## 建议拆分 PR

建议至少拆成以下 PR：

1. 新入口 `/clipal` + 协议识别内核
2. 新统一配置加载与旧配置兼容
3. adapter + probe + capability cache
4. runtime state 重构
5. Web UI 重构
6. 文档更新与迁移说明
