package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/lansespirit/Clipal/internal/config"
	"github.com/lansespirit/Clipal/internal/proxy"
	"github.com/lansespirit/Clipal/internal/testutil"
)

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

	got := testutil.DecodeJSONMap(t, w.Body.Bytes())
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
	if _, ok := got["ignore_count_tokens_failover"]; ok {
		t.Fatalf("did not expect ignore_count_tokens_failover in response")
	}
}

func TestHandleGetProviders_RedactsAPIKey_AndReturnsArray(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "openai.yaml"), []byte(`
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
	req := httptest.NewRequest(http.MethodGet, "/api/providers/openai", nil)
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
	if got[0]["key_count"] != float64(1) {
		t.Fatalf("expected key_count=1, got %v", got[0]["key_count"])
	}
}

func TestHandleExportConfig_IncludesAPIKey_SnakeCase(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "openai.yaml"), []byte(`
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

	got := testutil.DecodeJSONMap(t, w.Body.Bytes())
	openAIObj, ok := got["openai"].(map[string]any)
	if !ok {
		t.Fatalf("expected openai object, got %T", got["openai"])
	}
	providers, ok := openAIObj["providers"].([]any)
	if !ok || len(providers) != 1 {
		t.Fatalf("expected openai.providers array of len 1, got %T len=%d", openAIObj["providers"], len(providers))
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
  }
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
}

func TestHandleAddProvider_AcceptsAPIKeys(t *testing.T) {
	dir := t.TempDir()
	api := NewAPI(dir, "test", nil)

	body := []byte(`{
  "name": "p1",
  "base_url": "https://example.com",
  "api_keys": ["key1", "key2"],
  "priority": 1,
  "enabled": true
}`)

	req := httptest.NewRequest(http.MethodPost, "/api/providers/codex", bytes.NewReader(body))
	w := httptest.NewRecorder()
	api.HandleAddProvider(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Result().StatusCode, w.Body.String())
	}

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if got := cfg.OpenAI.Providers[0].KeyCount(); got != 2 {
		t.Fatalf("key count: got %d want %d", got, 2)
	}
	if cfg.OpenAI.Providers[0].APIKey != "" {
		t.Fatalf("expected multi-key provider to be persisted via api_keys")
	}
}

func TestHandleAddProvider_AutoAssignsNextPriority(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "openai.yaml"), []byte(`
providers:
  - name: p1
    base_url: https://one.example
    api_key: key1
    priority: 1
  - name: p2
    base_url: https://two.example
    api_key: key2
    priority: 2
`), 0o600); err != nil {
		t.Fatal(err)
	}

	api := NewAPI(dir, "test", nil)
	body := []byte(`{
  "name": "p3",
  "base_url": "https://three.example",
  "api_key": "key3",
  "enabled": true
}`)

	req := httptest.NewRequest(http.MethodPost, "/api/providers/codex", bytes.NewReader(body))
	w := httptest.NewRecorder()
	api.HandleAddProvider(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Result().StatusCode, w.Body.String())
	}

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(cfg.OpenAI.Providers) != 3 {
		t.Fatalf("providers len = %d, want 3", len(cfg.OpenAI.Providers))
	}
	if got := cfg.OpenAI.Providers[2].Priority; got != 3 {
		t.Fatalf("priority = %d, want 3", got)
	}
}

func TestHandleGetClientConfig_ReturnsConfiguredModeAndPin(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "openai.yaml"), []byte(`
mode: manual
pinned_provider: p1
providers:
  - name: p1
    base_url: https://example.com
    api_key: secret
    priority: 1
`), 0o600); err != nil {
		t.Fatal(err)
	}

	api := NewAPI(dir, "test", nil)
	req := httptest.NewRequest(http.MethodGet, "/api/client-config/openai", nil)
	w := httptest.NewRecorder()
	api.HandleGetClientConfig(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Result().StatusCode, w.Body.String())
	}

	got := testutil.DecodeJSONMap(t, w.Body.Bytes())
	if got["mode"] != "manual" || got["pinned_provider"] != "p1" {
		t.Fatalf("body=%#v", got)
	}
}

func TestHandleUpdateClientConfig_SavesChanges(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "openai.yaml"), []byte(`
mode: auto
providers:
  - name: p1
    base_url: https://example.com
    api_key: secret
    priority: 1
`), 0o600); err != nil {
		t.Fatal(err)
	}

	api := NewAPI(dir, "test", nil)
	req := httptest.NewRequest(http.MethodPut, "/api/client-config/codex", bytes.NewReader([]byte(`{"mode":"manual","pinned_provider":"p1"}`)))
	w := httptest.NewRecorder()
	api.HandleUpdateClientConfig(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Result().StatusCode, w.Body.String())
	}

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if cfg.OpenAI.Mode != config.ClientModeManual || cfg.OpenAI.PinnedProvider != "p1" {
		t.Fatalf("codex cfg = %#v", cfg.OpenAI)
	}
}

func TestHandleUpdateProvider_Paths(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "openai.yaml"), []byte(`
mode: auto
providers:
  - name: p1
    base_url: https://one.example
    api_key: key1
    priority: 1
  - name: p2
    base_url: https://two.example
    api_key: key2
    priority: 2
`), 0o600); err != nil {
		t.Fatal(err)
	}
	api := NewAPI(dir, "test", nil)

	t.Run("rename conflict", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/providers/codex/p1", bytes.NewReader([]byte(`{"name":"p2"}`)))
		w := httptest.NewRecorder()
		api.HandleUpdateProvider(w, req)
		if w.Result().StatusCode != http.StatusConflict {
			t.Fatalf("status=%d body=%s", w.Result().StatusCode, w.Body.String())
		}
	})

	t.Run("not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/providers/codex/missing", bytes.NewReader([]byte(`{"base_url":"https://new.example"}`)))
		w := httptest.NewRecorder()
		api.HandleUpdateProvider(w, req)
		if w.Result().StatusCode != http.StatusNotFound {
			t.Fatalf("status=%d body=%s", w.Result().StatusCode, w.Body.String())
		}
	})

	t.Run("success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/providers/codex/p1", bytes.NewReader([]byte(`{
  "name":"p3",
  "base_url":"https://three.example",
  "api_keys":["k3","k4"],
  "priority":5,
  "enabled":false
}`)))
		w := httptest.NewRecorder()
		api.HandleUpdateProvider(w, req)
		if w.Result().StatusCode != http.StatusOK {
			t.Fatalf("status=%d body=%s", w.Result().StatusCode, w.Body.String())
		}

		cfg, err := config.Load(dir)
		if err != nil {
			t.Fatalf("config.Load: %v", err)
		}
		var updated *config.Provider
		for i := range cfg.OpenAI.Providers {
			if cfg.OpenAI.Providers[i].Name == "p3" {
				updated = &cfg.OpenAI.Providers[i]
				break
			}
		}
		if updated == nil {
			t.Fatalf("providers=%#v", cfg.OpenAI.Providers)
		}
		if updated.BaseURL != "https://three.example" {
			t.Fatalf("provider=%#v", updated)
		}
		if updated.Priority != 5 {
			t.Fatalf("priority=%d", updated.Priority)
		}
		if updated.Enabled == nil || *updated.Enabled {
			t.Fatalf("enabled=%v", updated.Enabled)
		}
		if got := updated.KeyCount(); got != 2 {
			t.Fatalf("key count=%d", got)
		}
	})
}

func TestHandleDeleteProvider_Paths(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "openai.yaml"), []byte(`
mode: auto
providers:
  - name: p1
    base_url: https://one.example
    api_key: key1
    priority: 1
  - name: p2
    base_url: https://two.example
    api_key: key2
    priority: 2
`), 0o600); err != nil {
		t.Fatal(err)
	}
	api := NewAPI(dir, "test", nil)

	req := httptest.NewRequest(http.MethodDelete, "/api/providers/codex/missing", nil)
	w := httptest.NewRecorder()
	api.HandleDeleteProvider(w, req)
	if w.Result().StatusCode != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", w.Result().StatusCode, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/providers/codex/p1", nil)
	w = httptest.NewRecorder()
	api.HandleDeleteProvider(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Result().StatusCode, w.Body.String())
	}

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if len(cfg.OpenAI.Providers) != 1 || cfg.OpenAI.Providers[0].Name != "p2" {
		t.Fatalf("providers=%#v", cfg.OpenAI.Providers)
	}
}

func TestHandleReorderProviders_Paths(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "openai.yaml"), []byte(`
mode: auto
providers:
  - name: p1
    base_url: https://one.example
    api_key: key1
    priority: 1
  - name: p2
    base_url: https://two.example
    api_key: key2
    priority: 2
  - name: p3
    base_url: https://three.example
    api_key: key3
    priority: 3
`), 0o600); err != nil {
		t.Fatal(err)
	}
	api := NewAPI(dir, "test", nil)

	req := httptest.NewRequest(http.MethodPut, "/api/providers/codex/_reorder", bytes.NewReader([]byte(`{"providers":["missing"]}`)))
	w := httptest.NewRecorder()
	api.HandleReorderProviders(w, req)
	if w.Result().StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Result().StatusCode, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodPut, "/api/providers/codex/_reorder", bytes.NewReader([]byte(`{"providers":["p3","p1"]}`)))
	w = httptest.NewRecorder()
	api.HandleReorderProviders(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Result().StatusCode, w.Body.String())
	}

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if got := []string{cfg.OpenAI.Providers[0].Name, cfg.OpenAI.Providers[1].Name, cfg.OpenAI.Providers[2].Name}; strings.Join(got, ",") != "p3,p1,p2" {
		t.Fatalf("providers order=%v", got)
	}
	if got := []int{cfg.OpenAI.Providers[0].Priority, cfg.OpenAI.Providers[1].Priority, cfg.OpenAI.Providers[2].Priority}; got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Fatalf("priorities=%v", got)
	}
}

func TestHandleGetStatus_FallsBackToFirstEnabledProviderWithoutRuntime(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "openai.yaml"), []byte(`
mode: auto
providers:
  - name: p1
    base_url: https://one.example
    api_key: key1
    priority: 1
  - name: p2
    base_url: https://two.example
    api_key: key2
    priority: 2
    enabled: false
`), 0o600); err != nil {
		t.Fatal(err)
	}

	api := NewAPI(dir, "test-version", nil)
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	w := httptest.NewRecorder()
	api.HandleGetStatus(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Result().StatusCode, w.Body.String())
	}

	got := testutil.DecodeJSONMap(t, w.Body.Bytes())
	if got["version"] != "test-version" {
		t.Fatalf("body=%#v", got)
	}
	clients, ok := got["clients"].(map[string]any)
	if !ok {
		t.Fatalf("clients=%T %#v", got["clients"], got["clients"])
	}
	openAI, ok := clients["openai"].(map[string]any)
	if !ok {
		t.Fatalf("openai=%T %#v", clients["openai"], clients["openai"])
	}
	if openAI["current_provider"] != "p1" {
		t.Fatalf("openai status=%#v", openAI)
	}
	if openAI["provider_count"] != float64(2) {
		t.Fatalf("openai status=%#v", openAI)
	}
	if _, ok := openAI["current_providers"].(map[string]any); !ok {
		t.Fatalf("openai current_providers=%T %#v", openAI["current_providers"], openAI["current_providers"])
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

func TestBuildClientStatus_IncludesLastRequestOutcome(t *testing.T) {
	cc := config.ClientConfig{
		Mode: config.ClientModeAuto,
		Providers: []config.Provider{
			{Name: "p1", Priority: 1},
		},
	}
	now := time.Date(2026, 3, 18, 16, 32, 24, 0, time.UTC)
	rt := proxy.ClientRuntimeSnapshot{
		CurrentProvider: "p1",
		LastRequest: &proxy.RequestOutcomeEvent{
			At:       now,
			Provider: "p1",
			Status:   200,
			Delivery: "committed_complete",
			Protocol: "completed",
			Cause:    "",
			Bytes:    123,
		},
	}

	got := buildClientStatus(cc, cc.Providers, rt)
	if got.LastRequest == nil {
		t.Fatalf("expected last_request to be populated")
	}
	if got.LastRequest.Provider != "p1" {
		t.Fatalf("last_request.provider: got %q want %q", got.LastRequest.Provider, "p1")
	}
	if got.LastRequest.Delivery != "committed_complete" {
		t.Fatalf("last_request.delivery: got %q want %q", got.LastRequest.Delivery, "committed_complete")
	}
	if got.LastRequest.Protocol != "completed" {
		t.Fatalf("last_request.protocol: got %q want %q", got.LastRequest.Protocol, "completed")
	}
	if got.LastRequest.Bytes != 123 {
		t.Fatalf("last_request.bytes: got %d want %d", got.LastRequest.Bytes, 123)
	}
	if got.LastRequest.At != now.Format(time.RFC3339) {
		t.Fatalf("last_request.at: got %q want %q", got.LastRequest.At, now.Format(time.RFC3339))
	}
	if got.LastRequest.Result != "completed" {
		t.Fatalf("last_request.result: got %q want %q", got.LastRequest.Result, "completed")
	}
	if got.LastRequest.Label != "Completed via p1" {
		t.Fatalf("last_request.label: got %q want %q", got.LastRequest.Label, "Completed via p1")
	}
	if got.LastRequest.Detail == "" {
		t.Fatalf("expected last_request.detail to be populated")
	}
}

func TestBuildClientStatus_ReflectsLastRequestCapability(t *testing.T) {
	cc := config.ClientConfig{
		Mode: config.ClientModeAuto,
		Providers: []config.Provider{
			{Name: "p1", Priority: 1},
		},
	}
	now := time.Date(2026, 3, 18, 16, 32, 24, 0, time.UTC)
	lastRequest := &proxy.RequestOutcomeEvent{
		At:       now,
		Provider: "p1",
		Status:   200,
		Delivery: "committed_complete",
		Protocol: "completed",
		Bytes:    123,
	}
	inputField := reflect.ValueOf(lastRequest).Elem().FieldByName("Capability")
	if !inputField.IsValid() {
		t.Fatalf("expected RequestOutcomeEvent to expose Capability")
	}
	inputField.SetString("openai_responses")

	rt := proxy.ClientRuntimeSnapshot{
		CurrentProvider: "p1",
		LastRequest:     lastRequest,
	}

	got := buildClientStatus(cc, cc.Providers, rt)
	if got.LastRequest == nil {
		t.Fatalf("expected last_request to be populated")
	}

	outputField := reflect.ValueOf(*got.LastRequest).FieldByName("Capability")
	if !outputField.IsValid() {
		t.Fatalf("expected RequestOutcomeStatus to expose Capability")
	}
	if gotCapability := outputField.String(); gotCapability != "openai_responses" {
		t.Fatalf("last_request.capability: got %q want %q", gotCapability, "openai_responses")
	}
}

func TestBuildClientStatus_HidesCountTokensCapabilityFromUserFacingStatus(t *testing.T) {
	cc := config.ClientConfig{
		Mode: config.ClientModeAuto,
		Providers: []config.Provider{
			{Name: "p1", Priority: 1},
		},
	}
	now := time.Date(2026, 3, 18, 16, 32, 24, 0, time.UTC)
	lastRequest := &proxy.RequestOutcomeEvent{
		At:       now,
		Provider: "p1",
		Status:   200,
		Delivery: "committed_complete",
		Protocol: "completed",
		Bytes:    64,
	}
	inputField := reflect.ValueOf(lastRequest).Elem().FieldByName("Capability")
	if !inputField.IsValid() {
		t.Fatalf("expected RequestOutcomeEvent to expose Capability")
	}
	inputField.SetString("claude_count_tokens")

	rt := proxy.ClientRuntimeSnapshot{
		CurrentProvider: "p1",
		LastRequest:     lastRequest,
	}

	got := buildClientStatus(cc, cc.Providers, rt)
	if got.LastRequest == nil {
		t.Fatalf("expected last_request to be populated")
	}

	outputField := reflect.ValueOf(*got.LastRequest).FieldByName("Capability")
	if !outputField.IsValid() {
		t.Fatalf("expected RequestOutcomeStatus to expose Capability")
	}
	if gotCapability := outputField.String(); gotCapability != "" {
		t.Fatalf("last_request.capability: got %q want empty", gotCapability)
	}
}

func TestBuildClientStatus_NormalizesDisplayFields(t *testing.T) {
	cc := config.ClientConfig{
		Mode: config.ClientModeAuto,
		Providers: []config.Provider{
			{Name: "p1", Priority: 1},
			{Name: "p2", Priority: 2},
		},
	}
	now := time.Date(2026, 3, 18, 16, 32, 24, 0, time.UTC)
	deactivatedUntil := time.Now().Add(30 * time.Second)
	rt := proxy.ClientRuntimeSnapshot{
		CurrentProvider: "p2",
		LastSwitch: &proxy.ProviderSwitchEvent{
			At:     now,
			From:   "p1",
			To:     "p2",
			Reason: "rate_limit",
			Status: 429,
		},
		Providers: []proxy.ProviderRuntimeSnapshot{
			{
				Name:              "p1",
				DeactivatedReason: "rate_limit",
				DeactivatedUntil:  deactivatedUntil,
			},
			{
				Name:         "p2",
				CircuitState: "half_open",
			},
		},
	}

	got := buildClientStatus(cc, cc.Providers, rt)
	if got.LastSwitch == nil {
		t.Fatalf("expected last_switch to be populated")
	}
	if got.LastSwitch.Label != "p1 -> p2" {
		t.Fatalf("last_switch.label: got %q want %q", got.LastSwitch.Label, "p1 -> p2")
	}
	if got.LastSwitch.Detail == "" {
		t.Fatalf("expected last_switch.detail to be populated")
	}
	if len(got.Providers) != 2 {
		t.Fatalf("providers len: got %d want %d", len(got.Providers), 2)
	}
	if got.Providers[0].State != "cooling_down" {
		t.Fatalf("provider[0].state: got %q want %q", got.Providers[0].State, "cooling_down")
	}
	if got.Providers[0].Label != "p1 (cooling down)" {
		t.Fatalf("provider[0].label: got %q want %q", got.Providers[0].Label, "p1 (cooling down)")
	}
	if got.Providers[0].Detail == "" {
		t.Fatalf("expected provider[0].detail to be populated")
	}
	if got.Providers[1].State != "recovery_probe" {
		t.Fatalf("provider[1].state: got %q want %q", got.Providers[1].State, "recovery_probe")
	}
}

func TestBuildClientStatus_IncludesTerminalFailureOutcome(t *testing.T) {
	cc := config.ClientConfig{
		Mode: config.ClientModeAuto,
		Providers: []config.Provider{
			{Name: "p1", Priority: 1},
		},
	}
	now := time.Date(2026, 3, 18, 16, 32, 24, 0, time.UTC)
	rt := proxy.ClientRuntimeSnapshot{
		CurrentProvider: "p1",
		LastRequest: &proxy.RequestOutcomeEvent{
			At:       now,
			Provider: "p1",
			Status:   http.StatusServiceUnavailable,
			Result:   "all_providers_failed",
			Detail:   "p1 returned HTTP 503 Service Unavailable",
		},
	}

	got := buildClientStatus(cc, cc.Providers, rt)
	if got.LastRequest == nil {
		t.Fatalf("expected last_request to be populated")
	}
	if got.LastRequest.Result != "all_providers_failed" {
		t.Fatalf("last_request.result: got %q want %q", got.LastRequest.Result, "all_providers_failed")
	}
	if got.LastRequest.Label != "All providers failed" {
		t.Fatalf("last_request.label: got %q want %q", got.LastRequest.Label, "All providers failed")
	}
	if got.LastRequest.Detail != "p1 returned HTTP 503 Service Unavailable" {
		t.Fatalf("last_request.detail: got %q", got.LastRequest.Detail)
	}
}

func TestBuildClientStatus_IncludesRequestRejectedOutcome(t *testing.T) {
	cc := config.ClientConfig{
		Mode: config.ClientModeAuto,
		Providers: []config.Provider{
			{Name: "broken", Priority: 1},
		},
	}
	now := time.Date(2026, 3, 18, 16, 32, 24, 0, time.UTC)
	rt := proxy.ClientRuntimeSnapshot{
		CurrentProvider: "broken",
		LastRequest: &proxy.RequestOutcomeEvent{
			At:       now,
			Provider: "broken",
			Status:   http.StatusBadGateway,
			Result:   "request_rejected",
			Detail:   "broken request could not be prepared locally: invalid base_url",
		},
	}

	got := buildClientStatus(cc, cc.Providers, rt)
	if got.LastRequest == nil {
		t.Fatalf("expected last_request to be populated")
	}
	if got.LastRequest.Result != "request_rejected" {
		t.Fatalf("last_request.result: got %q want %q", got.LastRequest.Result, "request_rejected")
	}
	if got.LastRequest.Label != "Request rejected by proxy" {
		t.Fatalf("last_request.label: got %q want %q", got.LastRequest.Label, "Request rejected by proxy")
	}
}

func TestBuildClientStatus_ReportsNoAvailableKeys(t *testing.T) {
	cc := config.ClientConfig{
		Mode: config.ClientModeManual,
		Providers: []config.Provider{
			{Name: "p1", Priority: 1, APIKeys: []string{"k1", "k2"}},
		},
	}
	rt := proxy.ClientRuntimeSnapshot{
		CurrentProvider: "p1",
		Providers: []proxy.ProviderRuntimeSnapshot{
			{
				Name:              "p1",
				KeyCount:          2,
				AvailableKeyCount: 0,
			},
		},
	}

	got := buildClientStatus(cc, cc.Providers, rt)
	if len(got.Providers) != 1 {
		t.Fatalf("providers len: got %d want %d", len(got.Providers), 1)
	}
	if got.Providers[0].State != "unavailable" {
		t.Fatalf("provider.state: got %q want %q", got.Providers[0].State, "unavailable")
	}
	if got.Providers[0].SkipReason != "keys_exhausted" {
		t.Fatalf("provider.skip_reason: got %q want %q", got.Providers[0].SkipReason, "keys_exhausted")
	}
	if got.Providers[0].AvailableKeyCount != 0 {
		t.Fatalf("provider.available_key_count: got %d want %d", got.Providers[0].AvailableKeyCount, 0)
	}
}

func TestBuildClientStatus_ExposesScopeSpecificCurrentProviders(t *testing.T) {
	cc := config.ClientConfig{
		Mode: config.ClientModeAuto,
		Providers: []config.Provider{
			{Name: "p1", Priority: 1},
			{Name: "p2", Priority: 2},
		},
	}
	rt := proxy.ClientRuntimeSnapshot{
		CurrentProvider: "p1",
		CurrentProviders: map[string]string{
			"default":                        "p1",
			"openai_responses":               "p2",
			"gemini_stream_generate_content": "p2",
		},
	}

	got := buildClientStatus(cc, cc.Providers, rt)
	if got.CurrentProvider != "p1" {
		t.Fatalf("current_provider: got %q want %q", got.CurrentProvider, "p1")
	}
	if len(got.CurrentProviders) != 3 {
		t.Fatalf("current_providers len: got %d want %d", len(got.CurrentProviders), 3)
	}
	if got.CurrentProviders["default"] != "p1" {
		t.Fatalf("current_providers[default]: got %q want %q", got.CurrentProviders["default"], "p1")
	}
	if got.CurrentProviders["openai_responses"] != "p2" {
		t.Fatalf("current_providers[openai_responses]: got %q want %q", got.CurrentProviders["openai_responses"], "p2")
	}
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
