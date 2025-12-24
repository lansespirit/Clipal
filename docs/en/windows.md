# Windows Guide (PowerShell / Task Scheduler / Service)

English: [docs/en/windows.md](windows.md) | 中文: [docs/zh/windows.md](../zh/windows.md)

## 1. Download

From GitHub Releases: `clipal-windows-amd64.exe`

Renaming it to `clipal.exe` is recommended.

## 2. Location and `PATH`

Put it under a stable directory, for example:

`C:\\Users\\<YOU>\\bin\\clipal.exe`

Then add `C:\\Users\\<YOU>\\bin` to your user `PATH`.

Verify:

```powershell
clipal.exe --version
```

## 3. Initialize config

Default config dir:

`%USERPROFILE%\\.clipal\\`

Copy templates from `examples\\`:

```powershell
New-Item -ItemType Directory -Force "$env:USERPROFILE\\.clipal" | Out-Null
Copy-Item .\\examples\\config.yaml "$env:USERPROFILE\\.clipal\\config.yaml" -Force
Copy-Item .\\examples\\claude-code.yaml "$env:USERPROFILE\\.clipal\\claude-code.yaml" -Force
Copy-Item .\\examples\\codex.yaml "$env:USERPROFILE\\.clipal\\codex.yaml" -Force
Copy-Item .\\examples\\gemini.yaml "$env:USERPROFILE\\.clipal\\gemini.yaml" -Force
```

Edit `api_key` with your real value.

## 4. Foreground run (first-time verification)

```powershell
clipal.exe --log-level debug
```

In another PowerShell, check health:

```powershell
Invoke-WebRequest http://127.0.0.1:3333/health | Select-Object -Expand Content
```

## 5. Background run (Task Scheduler, recommended)

### 5.1 Quiet stdout + file logging

In `%USERPROFILE%\\.clipal\\config.yaml`:

```yaml
log_stdout: false
log_retention_days: 7
```

Default log dir: `%USERPROFILE%\\.clipal\\logs\\`

### 5.2 Create a “Start at logon” task

Task Scheduler → Create Task:

- General: “Run with highest privileges” only if you need it
- Triggers: At log on
- Actions: Start a program
  - Program/script: `C:\\Users\\<YOU>\\bin\\clipal.exe`
  - Arguments: `--config-dir C:\\Users\\<YOU>\\.clipal`

Verify:

```powershell
Invoke-WebRequest http://127.0.0.1:3333/health | Select-Object -Expand Content
```

## 6. Run as a Windows Service (optional)

If you need it to run without logging in, you can use NSSM (Non-Sucking Service Manager) to wrap `clipal.exe` as a service:

- Application: `C:\\Users\\<YOU>\\bin\\clipal.exe`
- Arguments: `--config-dir C:\\Users\\<YOU>\\.clipal`

## 7. FAQ

- Port in use: change `port` in `config.yaml` or run with `--port 3334`
- Security: keep `listen_addr: 127.0.0.1`

