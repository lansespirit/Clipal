# macOS 使用指南（Intel / Apple Silicon）

English: [docs/en/macos.md](../en/macos.md) | 中文: [docs/zh/macos.md](macos.md)

## 1. 下载

从 GitHub Releases 下载对应文件：

- Apple Silicon（M 系列）：`clipal-darwin-arm64`
- Intel：`clipal-darwin-amd64`

下载后建议改名为 `clipal` 方便使用。

## 2. 放到一个固定位置

推荐两种方式（任选其一）：

**方式 A：放到 `~/bin`（最通用，不依赖 Homebrew）**

```bash
mkdir -p ~/bin
mv ~/Downloads/clipal-darwin-arm64 ~/bin/clipal
chmod +x ~/bin/clipal
```

把 `~/bin` 加入 PATH（如果你已经有就跳过）：

```bash
echo 'export PATH="$HOME/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
```

**方式 B：放到 Homebrew 路径（Apple Silicon 通常是 `/opt/homebrew/bin`）**

```bash
sudo mv ~/Downloads/clipal-darwin-arm64 /opt/homebrew/bin/clipal
sudo chmod +x /opt/homebrew/bin/clipal
```

## 3. 初始化配置

```bash
mkdir -p ~/.clipal
cp examples/config.yaml ~/.clipal/config.yaml
cp examples/claude-code.yaml ~/.clipal/claude-code.yaml
cp examples/codex.yaml ~/.clipal/codex.yaml
cp examples/gemini.yaml ~/.clipal/gemini.yaml
```

编辑 `~/.clipal/*.yaml` 把 `api_key`（以及需要的话 `base_url`）换成你自己的。

## 4. 前台启动（用于首次验证）

```bash
clipal --log-level debug
```

验证服务是否起来：

```bash
curl -fsS http://127.0.0.1:3333/health
```

## 5. 后台静默运行（launchd，推荐）

### 5.1 配置日志静默与落盘

`~/.clipal/config.yaml` 建议设置：

```yaml
log_stdout: false
log_retention_days: 7
# log_dir: ""  # 留空则默认 ~/.clipal/logs
```

日志目录默认：`~/.clipal/logs/`，按天文件：`clipal-YYYY-MM-DD.log`。

### 5.2 创建 LaunchAgent

你可以用两种方式（二选一）：

- **方式 A：使用内置命令（推荐）**：`clipal service install` 会自动生成 plist 并通过 `launchctl` 加载/启动
- **方式 B：手动创建 plist**：适合你想完全自定义 plist 内容的场景

#### 方式 A：内置命令（推荐）

```bash
# 查看运行状态
clipal status

# 安装并启动（会写入 ~/Library/LaunchAgents/com.lansespirit.clipal.plist）
clipal service install

# 查看状态
clipal service status
clipal service status --raw

# 重启 / 停止 / 卸载
clipal service restart
clipal service stop
clipal service uninstall
```

如果你之前已经安装过、需要覆盖更新 plist，用：

```bash
clipal service install --force
```

如果你的配置目录不在默认的 `~/.clipal`，加上：

```bash
clipal service install --config-dir /path/to/config
```

如需把 stdout/stderr 单独落盘（便于排查 panic/崩溃），可选：

```bash
clipal service install --stdout ~/.clipal/logs/launchd.out --stderr ~/.clipal/logs/launchd.err
```

#### 方式 B：手动创建 plist

创建文件 `~/Library/LaunchAgents/com.lansespirit.clipal.plist`：

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
  <dict>
    <key>Label</key>
    <string>com.lansespirit.clipal</string>

    <key>ProgramArguments</key>
    <array>
      <string>/Users/YOUR_USER/bin/clipal</string>
      <string>--config-dir</string>
      <string>/Users/YOUR_USER/.clipal</string>
    </array>

    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
  </dict>
</plist>
```

把里面的 `/Users/YOUR_USER/...` 换成你的实际路径（可用 `which clipal` / `echo $HOME` 确认）。

加载与启动：

```bash
launchctl bootstrap "gui/$(id -u)" ~/Library/LaunchAgents/com.lansespirit.clipal.plist
launchctl kickstart -k "gui/$(id -u)/com.lansespirit.clipal"
```

停止与卸载：

```bash
launchctl bootout "gui/$(id -u)" ~/Library/LaunchAgents/com.lansespirit.clipal.plist
```

### 5.3 查看运行与日志

```bash
curl -fsS http://127.0.0.1:3333/health
tail -n 200 ~/.clipal/logs/clipal-$(date +%F).log
```

如果你在 `com.lansespirit.clipal.plist` 里额外配置了：

- `StandardOutPath`（stdout）
- `StandardErrorPath`（stderr）

那么进程崩溃/panic 堆栈等“非正常退出”信息通常只会出现在 `stderr` 文件里（比按天滚动的 `clipal-YYYY-MM-DD.log` 更关键）。

## 6. 常见问题

- **启动后发现“有请求但我没打开 Claude Code”**：通常是 VS Code/Qoder 的 Claude Code 扩展后台进程在重试；可用 `lsof -nP -iTCP:3333` 定位。
- **安全建议**：默认 `listen_addr: 127.0.0.1` 只允许本机访问；不要随意改为 `0.0.0.0`，否则局域网内可能直接使用你的上游 key。
