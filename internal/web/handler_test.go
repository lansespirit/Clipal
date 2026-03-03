package web

import (
	"bytes"
	"net/http"
	"net/http/httptest"
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
  },
  "ignore_count_tokens_failover": false
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
