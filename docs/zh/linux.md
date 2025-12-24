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

编辑 `~/.clipal/*.yaml` 填好 `api_key`。

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
