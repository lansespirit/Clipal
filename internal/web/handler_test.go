package web

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLocalOnly_RejectsNonLoopbackRemote(t *testing.T) {
	dir := t.TempDir()
	h := NewHandler(dir, "test", nil)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
	req.Host = "localhost:3333"
	req.RemoteAddr = "8.8.8.8:12345"
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestLocalOnly_RejectsNonLocalHostHeader(t *testing.T) {
	dir := t.TempDir()
	h := NewHandler(dir, "test", nil)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "http://evil.example/", nil)
	req.Host = "evil.example:3333"
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestLocalOnly_APIStateChanging_RequiresUIHeader(t *testing.T) {
	dir := t.TempDir()
	h := NewHandler(dir, "test", nil)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

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
    "enabled": false,
    "min_level": "error",
    "provider_switch": true
  },
  "circuit_breaker": {
    "failure_threshold": 4,
    "success_threshold": 2,
    "open_timeout": "60s",
    "half_open_max_inflight": 1
  }
}`)

	req := httptest.NewRequest(http.MethodPut, "http://localhost/api/config/global/update", bytes.NewReader(body))
	req.Host = "localhost:3333"
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got status=%d body=%s", w.Code, w.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodPut, "http://localhost/api/config/global/update", bytes.NewReader(body))
	req2.Host = "localhost:3333"
	req2.RemoteAddr = "127.0.0.1:12345"
	req2.Header.Set("X-Clipal-UI", "1")
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()

	mux.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got status=%d body=%s", w2.Code, w2.Body.String())
	}
}

func TestServeIndex_ContentTypeAndNotFound(t *testing.T) {
	h := NewHandler(t.TempDir(), "test", nil)

	req := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
	w := httptest.NewRecorder()
	h.serveIndex(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("content-type=%q", got)
	}
	body := w.Body.String()
	for _, want := range []string{
		`/static/styles.css`,
		`/static/clipal-icon.svg`,
		`rel="icon"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("index body missing %q", want)
		}
	}

	req = httptest.NewRequest(http.MethodGet, "http://localhost/missing", nil)
	w = httptest.NewRecorder()
	h.serveIndex(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestServeStatic_ContentTypeAndNotFound(t *testing.T) {
	h := NewHandler(t.TempDir(), "test", nil)

	req := httptest.NewRequest(http.MethodGet, "http://localhost/static/app.js", nil)
	w := httptest.NewRecorder()
	h.serveStatic(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); got != "text/javascript; charset=utf-8" && got != "application/javascript" {
		t.Fatalf("content-type=%q", got)
	}

	req = httptest.NewRequest(http.MethodGet, "http://localhost/static/missing.js", nil)
	w = httptest.NewRecorder()
	h.serveStatic(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing status=%d body=%s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "http://localhost/static/styles.css", nil)
	w = httptest.NewRecorder()
	h.serveStatic(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("styles.css status=%d body=%s", w.Code, w.Body.String())
	}
	css := w.Body.String()
	for _, want := range []string{
		".pill-xs",
		".pill-sm",
		".pill-status-compact",
		".pill {",
		".badge {",
		".badge-xs",
	} {
		if !strings.Contains(css, want) {
			t.Fatalf("styles.css missing %q", want)
		}
	}
	req = httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
	w = httptest.NewRecorder()
	h.serveIndex(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("index status=%d body=%s", w.Code, w.Body.String())
	}
	index := w.Body.String()
	for _, want := range []string{
		`class="version-pill pill pill-xs"`,
		`class="metric-pill pill pill-status-compact integration-card-status"`,
		`class="priority-badge badge badge-xs badge-outline badge-mono"`,
		`class="pin-badge badge badge-xs badge-warning"`,
		`service-action-shell`,
		`service-action-layout`,
		`service-action-main-row`,
		`service-action-aside`,
		`settings-compact-grid`,
		`settings-panel`,
		`settings-page-header`,
		`Routing Strategy`,
	} {
		if !strings.Contains(index, want) {
			t.Fatalf("index missing %q", want)
		}
	}
	if strings.Contains(index, `class="settings-toolbar card"`) {
		t.Fatalf("index still contains old settings hero card")
	}
	providersPos := strings.Index(index, `id="tabbtn-providers"`)
	integrationsPos := strings.Index(index, `id="tabbtn-integrations"`)
	settingsPos := strings.Index(index, `id="tabbtn-settings"`)
	servicesPos := strings.Index(index, `id="tabbtn-services"`)
	if !(providersPos >= 0 && integrationsPos > providersPos && settingsPos > integrationsPos && servicesPos > settingsPos) {
		t.Fatalf("unexpected tab order: providers=%d integrations=%d settings=%d services=%d", providersPos, integrationsPos, settingsPos, servicesPos)
	}
}

func TestServeStatic_ServesBrandIconAndUpdatedLabels(t *testing.T) {
	h := NewHandler(t.TempDir(), "test", nil)

	req := httptest.NewRequest(http.MethodGet, "http://localhost/static/clipal-icon.svg", nil)
	w := httptest.NewRecorder()
	h.serveStatic(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("icon status=%d body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); got != "image/svg+xml" {
		t.Fatalf("icon content-type=%q", got)
	}
	iconBody := w.Body.String()
	for _, want := range []string{`<svg`, `fill="#000000"`, `stroke="#FFFFFF"`} {
		if !strings.Contains(iconBody, want) {
			t.Fatalf("icon body missing %q", want)
		}
	}

	req = httptest.NewRequest(http.MethodGet, "http://localhost/static/app.js", nil)
	w = httptest.NewRecorder()
	h.serveStatic(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("app.js status=%d body=%s", w.Code, w.Body.String())
	}
	js := w.Body.String()
	for _, want := range []string{
		`{ value: 'claude', label: 'Claude' }`,
		`{ value: 'openai', label: 'OpenAI' }`,
		`{ value: 'gemini', label: 'Gemini' }`,
		`return 'Gemini CLI';`,
		`return 'Continue';`,
		`return 'Aider';`,
		`return 'Goose';`,
		`serviceBusyAction: ''`,
		`serviceActionDisabledReason(action) {`,
		`return 'Service is already running';`,
		`return 'Service is not running';`,
		`sticky_sessions: {`,
		`busy_backpressure: {`,
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("app.js missing %q", want)
		}
	}
	for _, unwanted := range []string{
		`{ value: 'claude-code', label: 'Claude Code' }`,
		`{ value: 'codex', label: 'Codex' }`,
	} {
		if strings.Contains(js, unwanted) {
			t.Fatalf("app.js still contains %q", unwanted)
		}
	}
}

func TestLoopbackAndLocalhostHelpers(t *testing.T) {
	t.Parallel()

	if !isLoopbackRemote("[::1]:3333") {
		t.Fatalf("expected IPv6 loopback remote to pass")
	}
	if !isLoopbackRemote("127.0.0.1:3333") {
		t.Fatalf("expected IPv4 loopback remote to pass")
	}
	if isLoopbackRemote("192.168.1.10:3333") {
		t.Fatalf("expected non-loopback remote to fail")
	}

	for _, host := range []string{"", "localhost:3333", "foo.localhost:9999", "[::1]:4444", "127.0.0.1:5555"} {
		if !isLocalhostHost(host) {
			t.Fatalf("expected host %q to be accepted as localhost", host)
		}
	}
	if isLocalhostHost("example.com:3333") {
		t.Fatalf("expected example.com to be rejected")
	}
}

func TestLocalOnly_AllowsIPv6LoopbackAndEmptyHost(t *testing.T) {
	called := false
	h := NewHandler(t.TempDir(), "test", nil)
	handler := h.localOnly(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
	req.RemoteAddr = "[::1]:4321"
	req.Host = ""
	w := httptest.NewRecorder()

	handler(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if !called {
		t.Fatalf("expected wrapped handler to be called")
	}
}

func TestLocalOnly_APIStateChanging_RejectsWrongContentType(t *testing.T) {
	dir := t.TempDir()
	h := NewHandler(dir, "test", nil)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPut, "http://localhost/api/config/global/update", strings.NewReader(`{}`))
	req.Host = "localhost:3333"
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("X-Clipal-UI", "1")
	req.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestIntegrations_UIAndRouteAreRegistered(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	h := NewHandler(t.TempDir(), "test", nil)

	req := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
	w := httptest.NewRecorder()
	h.serveIndex(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("index status=%d body=%s", w.Code, w.Body.String())
	}
	index := w.Body.String()
	if !strings.Contains(index, "CLI Takeover") {
		t.Fatalf("index missing integrations tab: %s", index)
	}
	for _, want := range []string{
		"This only edits your user-level config file.",
		"Restart the client or open a new session after applying changes.",
		"Current file",
		"integrationSecondaryPreviewLabel",
		"integration-card-actions",
		"integration-card-header",
		"integration-card-heading",
		"integration-card-actions-right",
		"integration-action-primary",
		"integration-card-summary",
		"integration-card-summary-row",
		"integration-action-shell",
		"integration-action-tooltip",
	} {
		if !strings.Contains(index, want) {
			t.Fatalf("index missing %q: %s", want, index)
		}
	}

	req = httptest.NewRequest(http.MethodGet, "http://localhost/static/app.js", nil)
	w = httptest.NewRecorder()
	h.serveStatic(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("app.js status=%d body=%s", w.Code, w.Body.String())
	}
	js := w.Body.String()
	for _, want := range []string{
		"/api/integrations",
		"Claude Code",
		"Codex CLI",
		"OpenCode",
		"Restart the client or open a new session",
		"ANTHROPIC_AUTH_TOKEN is left untouched",
		`wire_api = "responses"`,
		"current_content",
		"planned_content",
		"backup_content",
		"backup_target_existed",
		"Already using Clipal",
		"No backup yet. Apply once before restore becomes available.",
		"Restore is only available while Clipal is active.",
		"Latest backup",
		"Original file did not exist before Clipal takeover.",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("app.js missing %q", want)
		}
	}
	if strings.Contains(js, "clipal-placeholder") {
		t.Fatalf("app.js should not suggest overwriting Claude auth token")
	}

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	req = httptest.NewRequest(http.MethodGet, "http://localhost/api/integrations", nil)
	req.Host = "localhost:3333"
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("route status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestLocalOnly_APIBodyLimit(t *testing.T) {
	h := NewHandler(t.TempDir(), "test", nil)
	called := false
	handler := h.localOnly(func(w http.ResponseWriter, r *http.Request) {
		called = true
		buf := make([]byte, maxAPIRequestBytes+1)
		_, err := r.Body.Read(buf)
		if err == nil {
			t.Fatalf("expected request body limit error")
		}
		writeError(w, err.Error(), http.StatusRequestEntityTooLarge)
	})

	req := httptest.NewRequest(http.MethodPost, "http://localhost/api/test", bytes.NewReader(bytes.Repeat([]byte("a"), maxAPIRequestBytes+1)))
	req.Host = "localhost:3333"
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("X-Clipal-UI", "1")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler(w, req)
	if !called {
		t.Fatalf("expected wrapped handler to be called")
	}
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}
