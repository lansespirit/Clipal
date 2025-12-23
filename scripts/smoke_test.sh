#!/bin/bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

require_cmd go
require_cmd curl

if command -v python3 >/dev/null 2>&1; then
  PY=python3
elif command -v python >/dev/null 2>&1; then
  PY=python
else
  echo "missing required command: python3 (or python)" >&2
  exit 1
fi

get_free_port() {
  "$PY" - <<'PY'
import socket
s = socket.socket()
s.bind(("127.0.0.1", 0))
port = s.getsockname()[1]
s.close()
print(port)
PY
}

wait_http_ok() {
  local url="$1"
  local tries="${2:-50}"
  local delay="${3:-0.1}"
  for _ in $(seq 1 "$tries"); do
    if curl -fsS "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep "$delay"
  done
  echo "timeout waiting for: $url" >&2
  return 1
}

tmpdir="$(mktemp -d "${TMPDIR:-/tmp}/clipal-smoke.XXXXXXXX")"
cfgdir="$tmpdir/config"
mkdir -p "$cfgdir"

upstream_port="$(get_free_port)"
dead_port="$(get_free_port)"
clipal_port="$(get_free_port)"

upstream_pid=""
clipal_pid=""

cleanup() {
  set +e
  if [[ -n "${clipal_pid:-}" ]]; then
    kill "$clipal_pid" >/dev/null 2>&1 || true
    wait "$clipal_pid" >/dev/null 2>&1 || true
  fi
  if [[ -n "${upstream_pid:-}" ]]; then
    kill "$upstream_pid" >/dev/null 2>&1 || true
    wait "$upstream_pid" >/dev/null 2>&1 || true
  fi
  rm -rf "$tmpdir" >/dev/null 2>&1 || true
}
trap cleanup EXIT

cat >"$tmpdir/upstream.go" <<'GO'
package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
)

type reply struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Query  string `json:"query"`
	Auth   string `json:"auth"`
	XAPI   string `json:"x_api_key"`
	Body   string `json:"body"`
}

func main() {
	port := os.Getenv("UPSTREAM_PORT")
	if port == "" {
		port = "18080"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(reply{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.RawQuery,
			Auth:   r.Header.Get("Authorization"),
			XAPI:   r.Header.Get("x-api-key"),
			Body:   string(b),
		})
	})

	addr := "127.0.0.1:" + port
	log.Printf("upstream listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
GO

cat >"$cfgdir/config.yaml" <<YAML
listen_addr: 127.0.0.1
port: $clipal_port
log_level: debug
reactivate_after: 1h
YAML

cat >"$cfgdir/codex.yaml" <<YAML
providers:
  - name: "dead"
    base_url: "http://127.0.0.1:$dead_port"
    api_key: "dead-key"
    priority: 1
    enabled: true
  - name: "upstream"
    base_url: "http://127.0.0.1:$upstream_port"
    api_key: "key2"
    priority: 2
    enabled: true
YAML

export UPSTREAM_PORT="$upstream_port"
go run "$tmpdir/upstream.go" >"$tmpdir/upstream.log" 2>&1 &
upstream_pid="$!"
wait_http_ok "http://127.0.0.1:$upstream_port/"

echo "building clipal..."
(cd "$repo_root" && go build -o "$tmpdir/clipal" ./cmd/clipal)

echo "starting clipal on 127.0.0.1:$clipal_port ..."
"$tmpdir/clipal" --config-dir "$cfgdir" --listen-addr 127.0.0.1 --port "$clipal_port" --log-level debug >"$tmpdir/clipal.log" 2>&1 &
clipal_pid="$!"
wait_http_ok "http://127.0.0.1:$clipal_port/health"

echo "test: /health"
curl -fsS "http://127.0.0.1:$clipal_port/health" | "$PY" -c 'import json,sys; d=json.load(sys.stdin); assert d.get("status")=="healthy", d; print("ok")'

echo "test: /codex prefix stripping + failover + auth override"
curl -fsS -X POST \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer original-client-token' \
  --data '{"hello":"world"}' \
  "http://127.0.0.1:$clipal_port/codex/test?x=1" \
  | "$PY" -c 'import json,sys; d=json.load(sys.stdin); assert d["path"]=="/test", d; assert d["query"]=="x=1", d; assert d["auth"]=="Bearer key2", d; assert d["body"]=="{\"hello\":\"world\"}", d; print("ok")'

if ! grep -q "switching to next provider" "$tmpdir/clipal.log" 2>/dev/null; then
  echo "expected failover log line not found in clipal.log" >&2
  echo "---- clipal.log ----" >&2
  tail -n 200 "$tmpdir/clipal.log" >&2 || true
  exit 1
fi

echo "test: hot reload provider api_key"
cat >"$cfgdir/codex.yaml" <<YAML
providers:
  - name: "upstream"
    base_url: "http://127.0.0.1:$upstream_port"
    api_key: "key3"
    priority: 1
    enabled: true
YAML

sleep 6
curl -fsS -X POST \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer original-client-token' \
  --data '{"hello":"again"}' \
  "http://127.0.0.1:$clipal_port/codex/test" \
  | "$PY" -c 'import json,sys; d=json.load(sys.stdin); assert d["auth"]=="Bearer key3", d; print("ok")'

echo ""
echo "smoke test passed"
echo "logs: $tmpdir/clipal.log $tmpdir/upstream.log"
