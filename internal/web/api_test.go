package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/lansespirit/Clipal/internal/config"
)

func decodeJSON(t *testing.T, body *bytes.Buffer) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal(body.Bytes(), &v); err != nil {
		t.Fatalf("decode json: %v\nbody=%s", err, body.String())
	}
	return v
}

func TestHandleGetGlobalConfig_ReturnsSnakeCase(t *testing.T) {
	dir := t.TempDir()
	api := NewAPI(dir, "test", nil)

	req := httptest.NewRequest(http.MethodGet, "/api/config/global", nil)
	w := httptest.NewRecorder()
	api.HandleGetGlobalConfig(w, req)
	res := w.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.StatusCode, w.Body.String())
	}

	got := decodeJSON(t, w.Body)
	if _, ok := got["listen_addr"]; !ok {
		t.Fatalf("expected listen_addr in response, got keys=%v", keys(got))
	}
	if _, ok := got["ListenAddr"]; ok {
		t.Fatalf("did not expect ListenAddr in response")
	}
	ntf, ok := got["notifications"].(map[string]any)
	if !ok {
		t.Fatalf("expected notifications object, got %T", got["notifications"])
	}
	if _, ok := ntf["min_level"]; !ok {
		t.Fatalf("expected notifications.min_level, got keys=%v", keys(ntf))
	}
	if _, ok := ntf["MinLevel"]; ok {
		t.Fatalf("did not expect notifications.MinLevel")
	}
	if v, ok := got["log_stdout"]; !ok || (v != true && v != false) {
		t.Fatalf("expected log_stdout boolean, got %v (%T)", got["log_stdout"], got["log_stdout"])
	}
}

func TestHandleGetProviders_RedactsAPIKey_AndReturnsArray(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "codex.yaml"), []byte(`
providers:
  - name: p1
    base_url: https://example.com
    api_key: secret
    priority: 2
  - name: p2
    base_url: https://example2.com
    api_key: secret2
    priority: 1
    enabled: false
`), 0600); err != nil {
		t.Fatal(err)
	}

	api := NewAPI(dir, "test", nil)
	req := httptest.NewRequest(http.MethodGet, "/api/providers/codex", nil)
	w := httptest.NewRecorder()
	api.HandleGetProviders(w, req)
	res := w.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.StatusCode, w.Body.String())
	}

	var got []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode json: %v\nbody=%s", err, w.Body.String())
	}
	if got == nil {
		t.Fatalf("expected JSON array, got null")
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(got))
	}
	if _, ok := got[0]["api_key"]; ok {
		t.Fatalf("did not expect api_key in provider listing")
	}
	if _, ok := got[0]["APIKey"]; ok {
		t.Fatalf("did not expect APIKey in provider listing")
	}
	if _, ok := got[0]["base_url"]; !ok {
		t.Fatalf("expected base_url in provider listing, got keys=%v", keys(got[0]))
	}
}

func TestHandleExportConfig_IncludesAPIKey_SnakeCase(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "codex.yaml"), []byte(`
providers:
  - name: p1
    base_url: https://example.com
    api_key: secret
    priority: 1
`), 0600); err != nil {
		t.Fatal(err)
	}

	api := NewAPI(dir, "test", nil)
	req := httptest.NewRequest(http.MethodGet, "/api/config/export", nil)
	w := httptest.NewRecorder()
	api.HandleExportConfig(w, req)
	res := w.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.StatusCode, w.Body.String())
	}

	got := decodeJSON(t, w.Body)
	codexObj, ok := got["codex"].(map[string]any)
	if !ok {
		t.Fatalf("expected codex object, got %T", got["codex"])
	}
	providers, ok := codexObj["providers"].([]any)
	if !ok || len(providers) != 1 {
		t.Fatalf("expected codex.providers array of len 1, got %T len=%d", codexObj["providers"], len(providers))
	}
	p0, ok := providers[0].(map[string]any)
	if !ok {
		t.Fatalf("expected provider object, got %T", providers[0])
	}
	if p0["api_key"] != "secret" {
		t.Fatalf("expected api_key=secret, got %v", p0["api_key"])
	}
	if _, ok := p0["base_url"]; !ok {
		t.Fatalf("expected base_url in export, got keys=%v", keys(p0))
	}
	if _, ok := p0["BaseURL"]; ok {
		t.Fatalf("did not expect BaseURL in export")
	}
}

func TestHandleUpdateGlobalConfig_AcceptsSnakeCaseNotifications(t *testing.T) {
	dir := t.TempDir()
	api := NewAPI(dir, "test", nil)

	body := []byte(`{
  "listen_addr": "127.0.0.1",
  "port": 3333,
  "log_level": "info",
  "reactivate_after": "10m",
  "upstream_idle_timeout": "1m",
  "max_request_body_bytes": 12345,
  "log_dir": "",
  "log_retention_days": 7,
  "log_stdout": true,
  "notifications": {
    "enabled": true,
    "min_level": "warn",
    "provider_switch": false
  },
  "circuit_breaker": {
    "failure_threshold": 4,
    "success_threshold": 2,
    "open_timeout": "60s",
    "half_open_max_inflight": 1
  },
  "ignore_count_tokens_failover": true
}`)

	req := httptest.NewRequest(http.MethodPut, "/api/config/global/update", bytes.NewReader(body))
	w := httptest.NewRecorder()
	api.HandleUpdateGlobalConfig(w, req)
	res := w.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.StatusCode, w.Body.String())
	}

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if !cfg.Global.Notifications.Enabled {
		t.Fatalf("expected notifications.enabled=true")
	}
	if cfg.Global.Notifications.MinLevel != config.LogLevelWarn {
		t.Fatalf("expected notifications.min_level=warn, got %q", cfg.Global.Notifications.MinLevel)
	}
	if cfg.Global.Notifications.ProviderSwitch == nil || *cfg.Global.Notifications.ProviderSwitch {
		t.Fatalf("expected notifications.provider_switch=false, got %v", cfg.Global.Notifications.ProviderSwitch)
	}
	if !cfg.Global.IgnoreCountTokensFailover {
		t.Fatalf("expected ignore_count_tokens_failover=true")
	}
}

func TestReorderProviders_PreservesUnmentioned_AndRejectsUnknown(t *testing.T) {
	in := []config.Provider{
		{Name: "a", Priority: 1},
		{Name: "b", Priority: 2},
		{Name: "c", Priority: 3},
	}
	out, err := reorderProviders(in, []string{"b"})
	if err != nil {
		t.Fatalf("reorder: %v", err)
	}
	if len(out) != 3 || out[0].Name != "b" || out[1].Name != "a" || out[2].Name != "c" {
		t.Fatalf("unexpected reorder result: %+v", out)
	}
	if out[0].Priority != 1 || out[1].Priority != 2 || out[2].Priority != 3 {
		t.Fatalf("expected priorities normalized, got %+v", out)
	}

	if _, err := reorderProviders(in, []string{"nope"}); err == nil {
		t.Fatalf("expected error for unknown provider")
	}
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
