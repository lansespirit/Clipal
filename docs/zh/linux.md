# Linux 使用指南（systemd / nohup）

English: [docs/en/linux.md](../en/linux.md) | 中文: [docs/zh/linux.md](linux.md)

## 1. 下载

从 GitHub Releases 下载：

- x86_64：`clipal-linux-amd64`
- ARM64：`clipal-linux-arm64`

下载后建议改名为 `clipal`。

## 2. 安装到 PATH

```bash
chmod +x ./clipal-linux-amd64
sudo mv ./clipal-linux-amd64 /usr/local/bin/clipal
```

确认：

```bash
clipal --version
```

## 3. 初始化配置

```bash
mkdir -p ~/.clipal
cp examples/config.yaml ~/.clipal/config.yaml
cp examples/claude-code.yaml ~/.clipal/claude-code.yaml
cp examples/codex.yaml ~/.clipal/codex.yaml
cp examples/gemini.yaml ~/.clipal/gemini.yaml
```

编辑 `~/.clipal/*.yaml` 填好 `api_key` 或 `api_keys`。

## 4. 前台启动（首次验证）

```bash
clipal --log-level debug
curl -fsS http://127.0.0.1:3333/health
```

## 5. 后台静默运行

### 5.1 方式 A：nohup（简单，但不推荐长期使用）

```bash
nohup clipal --log-level info >/dev/null 2>&1 &
```

建议在 `~/.clipal/config.yaml` 设置：

```yaml
log_stdout: false
log_retention_days: 7
```

### 5.2 方式 B：systemd user service（推荐：无需 root）

你可以用两种方式（二选一）：

- **方式 B1：使用内置命令（推荐）**：`clipal service install` 会自动生成 unit 并通过 `systemctl --user` 启用/启动
- **方式 B2：手动创建 unit**：适合你想完全自定义 unit 内容的场景

#### 方式 B1：内置命令（推荐）

```bash
# 查看运行状态
clipal status

# 安装并启用开机自启（登录后），并立即启动
clipal service install

# 查看状态
clipal service status
clipal service status --raw

# 重启 / 停止 / 卸载
clipal service restart
clipal service stop
clipal service uninstall
```

如果你之前已经安装过、需要覆盖更新 unit，用：

```bash
clipal service install --force
```

如果你的配置目录不在默认的 `~/.clipal`，加上：

```bash
clipal service install --config-dir /path/to/config
```

#### 方式 B2：手动创建 unit

创建 `~/.config/systemd/user/clipal.service`：

```ini
[Unit]
Description=clipal local proxy
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/clipal --config-dir %h/.clipal
Restart=always
RestartSec=2

[Install]
WantedBy=default.target
```

说明：

- 如果你把二进制放在 `$HOME/bin/clipal`，把 `ExecStart` 改为 `%h/bin/clipal --config-dir %h/.clipal`

启动并设置开机自启（登录后）：

```bash
systemctl --user daemon-reload
systemctl --user enable --now clipal.service
```

查看状态：

```bash
systemctl --user status clipal.service
curl -fsS http://127.0.0.1:3333/health
```

查看日志（两种任选）：

- clipal 自己写入：`~/.clipal/logs/clipal-YYYY-MM-DD.log`
- systemd journal：`journalctl --user -u clipal.service -e`

## 6. 常见问题

- 端口被占用：改 `~/.clipal/config.yaml` 的 `port` 或运行时 `--port 3334`
- 安全建议：保持 `listen_addr: 127.0.0.1`
