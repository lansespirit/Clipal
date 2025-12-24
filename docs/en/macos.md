# macOS Guide (Intel / Apple Silicon)

English: [docs/en/macos.md](macos.md) | 中文: [docs/zh/macos.md](../zh/macos.md)

## 1. Download

From GitHub Releases:

- Apple Silicon (M-series): `clipal-darwin-arm64`
- Intel: `clipal-darwin-amd64`

After downloading, renaming it to `clipal` is recommended.

## 2. Put the binary in a stable location

Choose one:

**Option A: `~/bin` (most universal; no Homebrew required)**

```bash
mkdir -p ~/bin
mv ~/Downloads/clipal-darwin-arm64 ~/bin/clipal
chmod +x ~/bin/clipal
```

Add `~/bin` to your `PATH` (skip if you already did):

```bash
echo 'export PATH="$HOME/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
```

**Option B: Homebrew path (Apple Silicon is usually `/opt/homebrew/bin`)**

```bash
sudo mv ~/Downloads/clipal-darwin-arm64 /opt/homebrew/bin/clipal
sudo chmod +x /opt/homebrew/bin/clipal
```

## 3. Initialize config

```bash
mkdir -p ~/.clipal
cp examples/config.yaml ~/.clipal/config.yaml
cp examples/claude-code.yaml ~/.clipal/claude-code.yaml
cp examples/codex.yaml ~/.clipal/codex.yaml
cp examples/gemini.yaml ~/.clipal/gemini.yaml
```

Edit `~/.clipal/*.yaml` and replace `api_key` (and `base_url` if needed) with your own.

## 4. Foreground run (first-time verification)

```bash
clipal --log-level debug
```

Verify health:

```bash
curl -fsS http://127.0.0.1:3333/health
```

## 5. Background run (launchd, recommended)

### 5.1 Quiet stdout + file logging

Suggested `~/.clipal/config.yaml`:

```yaml
log_stdout: false
log_retention_days: 7
# log_dir: ""  # empty means ~/.clipal/logs by default
```

Default log dir: `~/.clipal/logs/` (daily files: `clipal-YYYY-MM-DD.log`).

### 5.2 Create a LaunchAgent

Create `~/Library/LaunchAgents/com.lansespirit.clipal.plist`:

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

Replace `/Users/YOUR_USER/...` with your real path (use `which clipal` / `echo $HOME`).

Load and start:

```bash
launchctl bootstrap "gui/$(id -u)" ~/Library/LaunchAgents/com.lansespirit.clipal.plist
launchctl kickstart -k "gui/$(id -u)/com.lansespirit.clipal"
```

Stop and unload:

```bash
launchctl bootout "gui/$(id -u)" ~/Library/LaunchAgents/com.lansespirit.clipal.plist
```

### 5.3 Check status and logs

```bash
curl -fsS http://127.0.0.1:3333/health
tail -n 200 ~/.clipal/logs/clipal-$(date +%F).log
```

## 6. FAQ

- **“I see requests but I didn’t open Claude Code”**: often VS Code/Qoder extensions retry in the background; run `lsof -nP -iTCP:3333` to inspect.
- **Security**: keep `listen_addr: 127.0.0.1` unless you really want LAN access; exposing upstream keys can be risky.

