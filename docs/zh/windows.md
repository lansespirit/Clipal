# Windows 使用指南（PowerShell / 任务计划程序 / 服务）

English: [docs/en/windows.md](../en/windows.md) | 中文: [docs/zh/windows.md](windows.md)

## 1. 下载

从 GitHub Releases 下载：`clipal-windows-amd64.exe`

建议改名为 `clipal.exe`。

## 2. 放置位置与 PATH

推荐放到用户目录下的工具目录，例如：

`C:\\Users\\<YOU>\\bin\\clipal.exe`

然后把 `C:\\Users\\<YOU>\\bin` 加到环境变量 `PATH`（系统设置 → 环境变量 → 用户变量）。

验证：

```powershell
clipal.exe --version
```

## 3. 初始化配置

配置目录默认在：

`%USERPROFILE%\\.clipal\\`

你可以把仓库 `examples\\` 下的模板拷贝过去：

```powershell
New-Item -ItemType Directory -Force "$env:USERPROFILE\\.clipal" | Out-Null
Copy-Item .\\examples\\config.yaml "$env:USERPROFILE\\.clipal\\config.yaml" -Force
Copy-Item .\\examples\\claude-code.yaml "$env:USERPROFILE\\.clipal\\claude-code.yaml" -Force
Copy-Item .\\examples\\codex.yaml "$env:USERPROFILE\\.clipal\\codex.yaml" -Force
Copy-Item .\\examples\\gemini.yaml "$env:USERPROFILE\\.clipal\\gemini.yaml" -Force
```

编辑 `api_key` 为你的真实值。

## 4. 前台启动（首次验证）

```powershell
clipal.exe --log-level debug
```

另开一个 PowerShell 测健康检查：

```powershell
Invoke-WebRequest http://127.0.0.1:3333/health | Select-Object -Expand Content
```

## 5. 后台静默运行（推荐：任务计划程序）

### 5.1 配置日志静默与落盘

在 `%USERPROFILE%\\.clipal\\config.yaml` 里：

```yaml
log_stdout: false
log_retention_days: 7
```

日志默认目录：`%USERPROFILE%\\.clipal\\logs\\`

### 5.2 创建“登录时启动”任务

打开“任务计划程序” → “创建任务”：

- 常规：勾选“使用最高权限运行”（如果你需要绑定低端口/写受限目录才需要）
- 触发器：登录时
- 操作：启动程序
  - 程序或脚本：`C:\\Users\\<YOU>\\bin\\clipal.exe`
  - 添加参数：`--config-dir C:\\Users\\<YOU>\\.clipal`
- 条件/设置：按需

验证：

```powershell
Invoke-WebRequest http://127.0.0.1:3333/health | Select-Object -Expand Content
```

## 6. 作为 Windows Service（可选）

如果你希望“无需登录也运行”，可以用 NSSM（Non-Sucking Service Manager）把 `clipal.exe` 包装成服务：

- Application：`C:\\Users\\<YOU>\\bin\\clipal.exe`
- Arguments：`--config-dir C:\\Users\\<YOU>\\.clipal`

（NSSM 的安装与使用属于第三方工具范畴，按你的运维习惯选择即可。）

## 7. 常见问题

- 端口被占用：改 `config.yaml` 的 `port` 或运行时加 `--port 3334`
- 安全建议：保持 `listen_addr: 127.0.0.1`
- 看到 `Warning: config file ... permissive permissions (666), consider chmod 600`：
  - 这是类 Unix 的权限提示；Windows 上 `os.Stat().Mode().Perm()` 不等同于 NTFS ACL，常会显示为 `0666`，属于误报。
  - 新版本会在 Windows 禁用该检查；旧版本可忽略该 Warning。
  - 如需收紧配置文件权限，可用 `icacls` 只允许当前用户访问（按你的实际目录调整）：
    - `icacls "$env:USERPROFILE\\.clipal" /inheritance:r /grant:r "$($env:USERNAME):(OI)(CI)F"`
- 任务计划程序提示“权限/找不到配置”：
  - 确保任务的“运行用户”与配置目录（通常是 `%USERPROFILE%\\.clipal\\`）所属用户一致。
  - 在“添加参数”里显式传 `--config-dir C:\\Users\\<YOU>\\.clipal`，避免拿到错误的用户目录。
