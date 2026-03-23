# 快速开始

## 1. 下载与安装

从 [Releases](https://github.com/lansespirit/Clipal/releases) 下载对应平台的二进制，并放到你的 `PATH` 中。
当前稳定版：[`v0.7.0`](https://github.com/lansespirit/Clipal/releases/tag/v0.7.0)

平台细节：

- [macOS](macos.md)
- [Linux](linux.md)
- [Windows](windows.md)

确认版本：

```bash
clipal --version
```

## 2. 初始化配置

默认配置目录：

- macOS / Linux: `~/.clipal/`
- Windows: `%USERPROFILE%\\.clipal\\`

从仓库模板初始化：

```bash
mkdir -p ~/.clipal
cp examples/config.yaml ~/.clipal/config.yaml
cp examples/claude-code.yaml ~/.clipal/claude-code.yaml
cp examples/codex.yaml ~/.clipal/codex.yaml
cp examples/gemini.yaml ~/.clipal/gemini.yaml
```

直接查看模板：

- [../../examples/config.yaml](../../examples/config.yaml)
- [../../examples/claude-code.yaml](../../examples/claude-code.yaml)
- [../../examples/codex.yaml](../../examples/codex.yaml)
- [../../examples/gemini.yaml](../../examples/gemini.yaml)

然后编辑这些文件，填入你的上游 `base_url`、`api_key` 或 `api_keys`。

详细字段说明见 [配置参考](config-reference.md)。

## 3. 启动 Clipal

```bash
clipal
```

常见启动参数：

```bash
clipal --config-dir /path/to/config
clipal --listen-addr 127.0.0.1
clipal --port 3333
clipal --log-level debug
```

Clipal 默认会同时启动：

- 本地代理服务
- 本机 Web 管理界面

## 4. 验证运行状态

```bash
curl -fsS http://127.0.0.1:3333/health
clipal status
clipal status --json
```

浏览器打开：

```text
http://127.0.0.1:3333/
```

## 5. 接入你的客户端

Clipal 现在统一推荐的客户端入口是：

- `/clipal`

旧配置仍可继续使用兼容别名：

- `/claudecode`
- `/codex`
- `/gemini`

具体接入方式见 [客户端接入](client-setup.md)。

## 6. 下一步看什么

- 想用图形界面管理 provider：看 [Web UI 使用说明](web-ui.md)
- 想了解自动切换、手动锁定、多 Key：看 [路由与故障切换](routing-and-failover.md)
- 想配置后台服务和开机自启：看 [后台服务、状态与更新](services.md)
- 遇到问题：看 [排障与 FAQ](troubleshooting.md)
