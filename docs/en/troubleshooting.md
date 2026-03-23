# Troubleshooting

## `clipal status` Says Not Running

Check:

```bash
clipal status
curl -v http://127.0.0.1:3333/health
```

Common causes:

- Clipal is not started
- you changed the port but still check `3333`
- config validation failed and the process exited on startup

## Port Already In Use

Symptoms:

- `clipal status` reports port in use
- startup fails to bind the port

Fixes:

- change `port` in `config.yaml`
- or override it with `--port`

## Web UI Does Not Open

Check:

- you are opening `http://127.0.0.1:<port>/`
- Clipal is running
- you are accessing it locally, not from another machine

Remember:

- the Web UI is localhost-only
- even if the proxy listens on `0.0.0.0`, the management UI is still not exposed remotely

## The Client Still Talks To The Official API Host

Common causes:

- the client config was not saved
- another config file is overriding your Base URL
- the client is not pointing at `http://127.0.0.1:3333/clipal`
- you are still using an outdated compatibility alias or official API host in another config file

Re-check [Client Setup](client-setup.md).

## Providers Keep Switching Or Always Fail

Check:

- whether `base_url` is correct
- whether `api_key` / `api_keys` are valid
- whether the provider is being skipped because of `401` / `402` / `403` / `429`
- whether all keys for that provider are currently unavailable

If you use the Web UI, inspect the provider state, available key count, and recent switch details there.

## Running In Background But No Logs

Check:

- `log_dir` in `config.yaml`
- whether `log_stdout` is set to `false`
- write permissions for `<config-dir>/logs/`

For long-running setups, prefer Clipal's rotating logs instead of relying only on service-manager stdout.

## Windows Permission Warning About Config Files

Older versions may show a permissive-permissions warning on Windows because Unix permission bits do not map cleanly to NTFS ACLs.

If everything else works, this warning is usually harmless.

## Still Stuck

Recommended order:

1. `clipal status`
2. `curl http://127.0.0.1:<port>/health`
3. open the Web UI and inspect provider state
4. check `~/.clipal/*.yaml`
5. inspect today's log file

Related docs:

- [Config Reference](config-reference.md)
- [Routing and Failover](routing-and-failover.md)
- [Services, Status, and Updates](services.md)
