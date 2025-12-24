# Linux Guide (systemd / nohup)

English: [docs/en/linux.md](linux.md) | 中文: [docs/zh/linux.md](../zh/linux.md)

## 1. Download

From GitHub Releases:

- x86_64: `clipal-linux-amd64`
- ARM64: `clipal-linux-arm64`

After downloading, renaming it to `clipal` is recommended.

## 2. Install to `PATH`

```bash
chmod +x ./clipal-linux-amd64
sudo mv ./clipal-linux-amd64 /usr/local/bin/clipal
```

Verify:

```bash
clipal --version
```

## 3. Initialize config

```bash
mkdir -p ~/.clipal
cp examples/config.yaml ~/.clipal/config.yaml
cp examples/claude-code.yaml ~/.clipal/claude-code.yaml
cp examples/codex.yaml ~/.clipal/codex.yaml
cp examples/gemini.yaml ~/.clipal/gemini.yaml
```

Edit `~/.clipal/*.yaml` and fill in `api_key`.

## 4. Foreground run (first-time verification)

```bash
clipal --log-level debug
curl -fsS http://127.0.0.1:3333/health
```

## 5. Background run

### 5.1 Option A: `nohup` (simple; not recommended for long-term)

```bash
nohup clipal --log-level info >/dev/null 2>&1 &
```

Suggested `~/.clipal/config.yaml`:

```yaml
log_stdout: false
log_retention_days: 7
```

### 5.2 Option B: systemd user service (recommended; no root)

Create `~/.config/systemd/user/clipal.service`:

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

If your binary is in `$HOME/bin/clipal`, change `ExecStart` to:

`ExecStart=%h/bin/clipal --config-dir %h/.clipal`

Enable and start (after login):

```bash
systemctl --user daemon-reload
systemctl --user enable --now clipal.service
```

Check:

```bash
systemctl --user status clipal.service
curl -fsS http://127.0.0.1:3333/health
```

Logs:

- Clipal files: `~/.clipal/logs/clipal-YYYY-MM-DD.log`
- systemd journal: `journalctl --user -u clipal.service -e`

## 6. FAQ

- Port in use: change `port` in `~/.clipal/config.yaml` or run with `--port 3334`
- Security: keep `listen_addr: 127.0.0.1`

