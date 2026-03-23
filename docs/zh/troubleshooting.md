# 排障与 FAQ

## `clipal status` 显示 Not running

先检查：

```bash
clipal status
curl -v http://127.0.0.1:3333/health
```

常见原因：

- Clipal 没有启动
- 端口改了，但你还在访问 `3333`
- 配置文件校验失败，启动时直接退出

## 端口被占用

表现：

- `clipal status` 提示 port in use
- 启动时报端口监听失败

处理：

- 改 `config.yaml` 的 `port`
- 或启动时显式指定 `--port`

## Web UI 打不开

检查：

- 你访问的是 `http://127.0.0.1:<port>/`
- Clipal 正在运行
- 你是在本机访问，而不是从其他机器访问

注意：

- Web UI 只允许 localhost / loopback
- 即使代理监听在 `0.0.0.0`，管理界面仍然不会对外开放

## 客户端接入后仍然请求官方地址

常见原因：

- 客户端配置没保存
- 还有别的配置文件覆盖了你的 Base URL
- 客户端 Base URL 没有指向 `http://127.0.0.1:3333/clipal`
- 还有旧的兼容别名或官方 API 地址残留在其他配置里

先核对 [客户端接入](client-setup.md)。

## provider 一直切换或总是失败

先看：

- provider 的 `base_url` 是否正确
- `api_key` / `api_keys` 是否有效
- 是否因为 `401` / `402` / `403` / `429` 被临时跳过
- 是否所有 key 都已经不可用

如果使用 Web UI，可以直接查看 provider 状态、可用 key 数和最近切换信息。

## 后台运行但没日志

建议检查：

- `config.yaml` 里的 `log_dir`
- `log_stdout` 是否设为 `false`
- `<config-dir>/logs/` 是否存在写权限

长期后台运行建议优先看轮转日志，而不是只看系统服务 stdout。

## Windows 上看到 permissive permissions 警告

这是旧版本在 Windows 上常见的误报，原因是类 Unix 权限位和 NTFS ACL 不是同一套语义。

如果功能正常，一般可以忽略；新版本会逐步减少这类干扰。

## 仍然没解决

排查顺序建议：

1. `clipal status`
2. `curl http://127.0.0.1:<port>/health`
3. 打开 Web UI 看 provider 状态
4. 检查 `~/.clipal/*.yaml`
5. 查看当天日志文件

相关文档：

- [配置参考](config-reference.md)
- [路由与故障切换](routing-and-failover.md)
- [后台服务、状态与更新](services.md)
