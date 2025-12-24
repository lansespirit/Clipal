package proxy

import (
	"bytes"
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lansespirit/Clipal/internal/config"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func newResponse(status int, hdr http.Header, body string) *http.Response {
	if hdr == nil {
		hdr = make(http.Header)
	}
	return &http.Response{
		StatusCode: status,
		Header:     hdr,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestPathPrefixMatchAndStrip(t *testing.T) {
	t.Parallel()

	if !pathMatchesPrefix("/claudecode", "/claudecode") {
		t.Fatalf("expected exact prefix match")
	}
	if !pathMatchesPrefix("/claudecode/v1/messages", "/claudecode") {
		t.Fatalf("expected subpath prefix match")
	}
	if pathMatchesPrefix("/claudecodeX/v1/messages", "/claudecode") {
		t.Fatalf("expected non-match for similar prefix")
	}

	if got := stripClientPrefix("/claudecode", "/claudecode"); got != "/" {
		t.Fatalf("expected '/', got %q", got)
	}
	if got := stripClientPrefix("/claudecode/", "/claudecode"); got != "/" {
		t.Fatalf("expected '/', got %q", got)
	}
	if got := stripClientPrefix("/claudecode/v1/messages", "/claudecode"); got != "/v1/messages" {
		t.Fatalf("expected '/v1/messages', got %q", got)
	}
}

func TestBuildTargetURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		baseURL   string
		path      string
		rawQuery  string
		wantURL   string
		wantError bool
	}{
		{
			name:    "scheme-present",
			baseURL: "https://api.anthropic.com",
			path:    "/v1/messages",
			wantURL: "https://api.anthropic.com/v1/messages",
		},
		{
			name:    "scheme-missing-defaults-to-https",
			baseURL: "api.anthropic.com",
			path:    "/v1/messages",
			wantURL: "https://api.anthropic.com/v1/messages",
		},
		{
			name:    "base-path-joined",
			baseURL: "https://example.com/api",
			path:    "/v1/messages",
			wantURL: "https://example.com/api/v1/messages",
		},
		{
			name:     "query-preserved",
			baseURL:  "https://example.com",
			path:     "/v1/messages",
			rawQuery: "a=1&b=2",
			wantURL:  "https://example.com/v1/messages?a=1&b=2",
		},
		{
			name:      "empty-base",
			baseURL:   "   ",
			path:      "/v1/messages",
			wantError: true,
			wantURL:   "",
			rawQuery:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := buildTargetURL(tc.baseURL, tc.path, tc.rawQuery)
			if tc.wantError {
				if err == nil {
					t.Fatalf("expected error, got nil (url=%q)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.wantURL {
				t.Fatalf("unexpected url: got %q want %q", got, tc.wantURL)
			}
		})
	}
}

func TestAddForwardedHeaders(t *testing.T) {
	t.Parallel()

	original, err := http.NewRequest(http.MethodPost, "http://localhost/claudecode/v1/messages", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	original.RemoteAddr = "1.2.3.4:5678"
	original.Host = "example.com:3333"
	original.TLS = &tls.ConnectionState{}

	proxyReq, err := http.NewRequest(http.MethodPost, "https://api.example.com/v1/messages", nil)
	if err != nil {
		t.Fatalf("new proxy request: %v", err)
	}

	addForwardedHeaders(proxyReq, original)

	if got := proxyReq.Header.Get("X-Forwarded-For"); got != "1.2.3.4" {
		t.Fatalf("X-Forwarded-For: got %q want %q", got, "1.2.3.4")
	}
	if got := proxyReq.Header.Get("X-Forwarded-Proto"); got != "https" {
		t.Fatalf("X-Forwarded-Proto: got %q want %q", got, "https")
	}
	if got := proxyReq.Header.Get("X-Forwarded-Host"); got != "example.com:3333" {
		t.Fatalf("X-Forwarded-Host: got %q want %q", got, "example.com:3333")
	}
	if got := proxyReq.Header.Get("X-Forwarded-Port"); got != "3333" {
		t.Fatalf("X-Forwarded-Port: got %q want %q", got, "3333")
	}
}

func TestHandleRequest_MaxRequestBodyBytes(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Global: config.GlobalConfig{
			ListenAddr:      "127.0.0.1",
			Port:            3333,
			LogLevel:        config.LogLevelDebug,
			ReactivateAfter: "1h",
			MaxRequestBody:  8,
		},
		Codex: config.ClientConfig{
			Providers: []config.Provider{
				{Name: "p1", BaseURL: "http://example.com", APIKey: "k1", Priority: 1},
			},
		},
	}

	router := NewRouter(cfg)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://proxy/codex/v1/test", bytes.NewReader([]byte("123456789")))
	router.handleRequest(rr, req)

	if rr.Result().StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status: got %d want %d", rr.Result().StatusCode, http.StatusRequestEntityTooLarge)
	}
}

func TestForwardWithFailover_DeactivateOn401(t *testing.T) {
	t.Parallel()

	rt := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Host {
		case "p1":
			h := make(http.Header)
			h.Set("Content-Type", "application/json")
			return newResponse(http.StatusUnauthorized, h, `{"error":{"type":"authentication_error","code":"invalid_api_key","message":"bad key"}}`), nil
		case "p2":
			return newResponse(http.StatusOK, nil, "ok"), nil
		default:
			return nil, errors.New("unexpected host")
		}
	})

	cp := newClientProxy(ClientCodex, []config.Provider{
		{Name: "p1", BaseURL: "http://p1", APIKey: "k1", Priority: 1},
		{Name: "p2", BaseURL: "http://p2", APIKey: "k2", Priority: 2},
	}, time.Hour)
	cp.httpClient.Transport = rt

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://proxy/codex/v1/test", bytes.NewReader([]byte(`{"x":1}`)))
	cp.forwardWithFailover(rr, req, "/v1/test")

	res := rr.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want %d", res.StatusCode, http.StatusOK)
	}
	if got := rr.Body.String(); got != "ok" {
		t.Fatalf("body: got %q want %q", got, "ok")
	}
	if !cp.isDeactivated(0) {
		t.Fatalf("expected provider 0 to be deactivated")
	}
	if got := cp.getCurrentIndex(); got != 1 {
		t.Fatalf("currentIndex: got %d want %d", got, 1)
	}
}

func TestForwardWithFailover_RetryOn503DoesNotDeactivate(t *testing.T) {
	t.Parallel()

	rt := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Host {
		case "p1":
			return newResponse(http.StatusServiceUnavailable, nil, "down"), nil
		case "p2":
			return newResponse(http.StatusOK, nil, "ok"), nil
		default:
			return nil, errors.New("unexpected host")
		}
	})

	cp := newClientProxy(ClientCodex, []config.Provider{
		{Name: "p1", BaseURL: "http://p1", APIKey: "k1", Priority: 1},
		{Name: "p2", BaseURL: "http://p2", APIKey: "k2", Priority: 2},
	}, time.Hour)
	cp.httpClient.Transport = rt

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://proxy/codex/v1/test", bytes.NewReader([]byte(`{"x":1}`)))
	cp.forwardWithFailover(rr, req, "/v1/test")

	res := rr.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want %d", res.StatusCode, http.StatusOK)
	}
	if got := rr.Body.String(); got != "ok" {
		t.Fatalf("body: got %q want %q", got, "ok")
	}
	if cp.isDeactivated(0) {
		t.Fatalf("expected provider 0 NOT to be deactivated")
	}
}

func TestClaudeCountTokens_IsolatedFailoverDoesNotChangeProvider(t *testing.T) {
	t.Parallel()

	var srv1Count int
	var srv2Count int

	rt := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Host {
		case "p1":
			srv1Count++
			return newResponse(http.StatusServiceUnavailable, nil, "down"), nil
		case "p2":
			srv2Count++
			return newResponse(http.StatusOK, nil, "ok"), nil
		default:
			return nil, errors.New("unexpected host")
		}
	})

	cfg := &config.Config{
		Global: config.GlobalConfig{
			ListenAddr:                "127.0.0.1",
			Port:                      3333,
			LogLevel:                  config.LogLevelDebug,
			ReactivateAfter:           "1h",
			IgnoreCountTokensFailover: true,
		},
		ClaudeCode: config.ClientConfig{
			Providers: []config.Provider{
				{Name: "p1", BaseURL: "http://p1", APIKey: "k1", Priority: 1},
				{Name: "p2", BaseURL: "http://p2", APIKey: "k2", Priority: 2},
			},
		},
	}

	router := NewRouter(cfg)
	cp := router.proxies[ClientClaudeCode]
	if cp == nil {
		t.Fatalf("expected claudecode proxy to be initialized")
	}
	if got := cp.getCurrentIndex(); got != 0 {
		t.Fatalf("currentIndex: got %d want %d", got, 0)
	}
	cp.httpClient.Transport = rt

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://proxy/claudecode/v1/messages/count_tokens", bytes.NewReader([]byte(`{"x":1}`)))
	router.handleRequest(rr, req)

	res := rr.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want %d", res.StatusCode, http.StatusOK)
	}
	if got := rr.Body.String(); got != "ok" {
		t.Fatalf("body: got %q want %q", got, "ok")
	}
	if got := cp.getCurrentIndex(); got != 0 {
		t.Fatalf("currentIndex changed by count_tokens: got %d want %d", got, 0)
	}
	if srv1Count != 1 || srv2Count != 1 {
		t.Fatalf("unexpected request counts: srv1=%d srv2=%d", srv1Count, srv2Count)
	}

	rr2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "http://proxy/claudecode/v1/messages/count_tokens", bytes.NewReader([]byte(`{"x":2}`)))
	router.handleRequest(rr2, req2)

	res2 := rr2.Result()
	if res2.StatusCode != http.StatusOK {
		t.Fatalf("status2: got %d want %d", res2.StatusCode, http.StatusOK)
	}
	if got := rr2.Body.String(); got != "ok" {
		t.Fatalf("body2: got %q want %q", got, "ok")
	}
	if got := cp.getCurrentIndex(); got != 0 {
		t.Fatalf("currentIndex changed by count_tokens (second): got %d want %d", got, 0)
	}
	if srv1Count != 1 || srv2Count != 2 {
		t.Fatalf("unexpected request counts after second call: srv1=%d srv2=%d", srv1Count, srv2Count)
	}
}

func TestForwardWithFailover_DeactivateOn429Quota(t *testing.T) {
	t.Parallel()

	rt := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Host {
		case "p1":
			h := make(http.Header)
			h.Set("Content-Type", "application/json")
			return newResponse(http.StatusTooManyRequests, h, `{"error":{"message":"quota","type":"insufficient_quota","code":"insufficient_quota"}}`), nil
		case "p2":
			return newResponse(http.StatusOK, nil, "ok"), nil
		default:
			return nil, errors.New("unexpected host")
		}
	})

	cp := newClientProxy(ClientCodex, []config.Provider{
		{Name: "p1", BaseURL: "http://p1", APIKey: "k1", Priority: 1},
		{Name: "p2", BaseURL: "http://p2", APIKey: "k2", Priority: 2},
	}, time.Hour)
	cp.httpClient.Transport = rt

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://proxy/codex/v1/test", bytes.NewReader([]byte(`{"x":1}`)))
	cp.forwardWithFailover(rr, req, "/v1/test")

	res := rr.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want %d", res.StatusCode, http.StatusOK)
	}
	if !cp.isDeactivated(0) {
		t.Fatalf("expected provider 0 to be deactivated")
	}
}

func TestForwardWithFailover_429RateLimitDoesNotDeactivate(t *testing.T) {
	t.Parallel()

	rt := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Host {
		case "p1":
			h := make(http.Header)
			h.Set("Content-Type", "application/json")
			return newResponse(http.StatusTooManyRequests, h, `{"error":{"message":"rate limit","type":"rate_limit_exceeded","code":"rate_limit_exceeded"}}`), nil
		case "p2":
			return newResponse(http.StatusOK, nil, "ok"), nil
		default:
			return nil, errors.New("unexpected host")
		}
	})

	cp := newClientProxy(ClientCodex, []config.Provider{
		{Name: "p1", BaseURL: "http://p1", APIKey: "k1", Priority: 1},
		{Name: "p2", BaseURL: "http://p2", APIKey: "k2", Priority: 2},
	}, time.Hour)
	cp.httpClient.Transport = rt

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://proxy/codex/v1/test", bytes.NewReader([]byte(`{"x":1}`)))
	cp.forwardWithFailover(rr, req, "/v1/test")

	res := rr.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want %d", res.StatusCode, http.StatusOK)
	}
	if cp.isDeactivated(0) {
		t.Fatalf("expected provider 0 NOT to be deactivated")
	}
}

func TestForwardWithFailover_ReactivateAfterTTL(t *testing.T) {
	t.Parallel()

	rt := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		return newResponse(http.StatusOK, nil, "ok"), nil
	})

	cp := newClientProxy(ClientCodex, []config.Provider{
		{Name: "p1", BaseURL: "http://p1", APIKey: "k1", Priority: 1},
	}, time.Hour)
	cp.httpClient.Transport = rt

	// Simulate a prior deactivation older than the TTL.
	cp.deactivated[0] = providerDeactivation{until: time.Now().Add(-2 * time.Hour)}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://proxy/codex/v1/test", bytes.NewReader([]byte(`{"x":1}`)))
	cp.forwardWithFailover(rr, req, "/v1/test")

	if cp.isDeactivated(0) {
		t.Fatalf("expected provider 0 to be reactivated")
	}
	if rr.Result().StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want %d", rr.Result().StatusCode, http.StatusOK)
	}
}

func TestForwardWithFailover_429AnthropicRateLimitDoesNotDeactivate(t *testing.T) {
	t.Parallel()

	rt := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Host {
		case "p1":
			h := make(http.Header)
			h.Set("Content-Type", "application/json")
			return newResponse(http.StatusTooManyRequests, h, `{"type":"error","error":{"type":"rate_limit_error","message":"rate limit"}}`), nil
		case "p2":
			return newResponse(http.StatusOK, nil, "ok"), nil
		default:
			return nil, errors.New("unexpected host")
		}
	})

	cp := newClientProxy(ClientClaudeCode, []config.Provider{
		{Name: "p1", BaseURL: "http://p1", APIKey: "k1", Priority: 1},
		{Name: "p2", BaseURL: "http://p2", APIKey: "k2", Priority: 2},
	}, time.Hour)
	cp.httpClient.Transport = rt

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://proxy/claudecode/v1/test", bytes.NewReader([]byte(`{"x":1}`)))
	cp.forwardWithFailover(rr, req, "/v1/test")

	if rr.Result().StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want %d", rr.Result().StatusCode, http.StatusOK)
	}
	if cp.isDeactivated(0) {
		t.Fatalf("expected provider 0 NOT to be deactivated")
	}
}

func TestForwardWithFailover_429RetryAfterCooldown(t *testing.T) {
	t.Parallel()

	rt := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Host {
		case "p1":
			h := make(http.Header)
			h.Set("Content-Type", "application/json")
			h.Set("Retry-After", "120")
			return newResponse(http.StatusTooManyRequests, h, `{"error":{"message":"rate limit","type":"rate_limit_exceeded","code":"rate_limit_exceeded"}}`), nil
		case "p2":
			return newResponse(http.StatusOK, nil, "ok"), nil
		default:
			return nil, errors.New("unexpected host")
		}
	})

	cp := newClientProxy(ClientCodex, []config.Provider{
		{Name: "p1", BaseURL: "http://p1", APIKey: "k1", Priority: 1},
		{Name: "p2", BaseURL: "http://p2", APIKey: "k2", Priority: 2},
	}, time.Hour)
	cp.httpClient.Transport = rt

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://proxy/codex/v1/test", bytes.NewReader([]byte(`{"x":1}`)))
	cp.forwardWithFailover(rr, req, "/v1/test")

	if rr.Result().StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want %d", rr.Result().StatusCode, http.StatusOK)
	}
	if !cp.isDeactivated(0) {
		t.Fatalf("expected provider 0 to be in cooldown")
	}
	until := cp.deactivationUntil(0)
	if until.IsZero() {
		t.Fatalf("expected non-zero cooldown until")
	}
	if until.Sub(time.Now()) < 90*time.Second {
		t.Fatalf("expected cooldown close to 120s, got %s", until.Sub(time.Now()))
	}
}

func TestForwardWithFailover_429RetryAfterCappedAtOneHour(t *testing.T) {
	t.Parallel()

	rt := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Host {
		case "p1":
			h := make(http.Header)
			h.Set("Content-Type", "application/json")
			h.Set("Retry-After", "7200")
			return newResponse(http.StatusTooManyRequests, h, `{"error":{"message":"rate limit","type":"rate_limit_exceeded","code":"rate_limit_exceeded"}}`), nil
		case "p2":
			return newResponse(http.StatusOK, nil, "ok"), nil
		default:
			return nil, errors.New("unexpected host")
		}
	})

	cp := newClientProxy(ClientCodex, []config.Provider{
		{Name: "p1", BaseURL: "http://p1", APIKey: "k1", Priority: 1},
		{Name: "p2", BaseURL: "http://p2", APIKey: "k2", Priority: 2},
	}, time.Hour)
	cp.httpClient.Transport = rt

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://proxy/codex/v1/test", bytes.NewReader([]byte(`{"x":1}`)))
	cp.forwardWithFailover(rr, req, "/v1/test")

	if rr.Result().StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want %d", rr.Result().StatusCode, http.StatusOK)
	}
	if !cp.isDeactivated(0) {
		t.Fatalf("expected provider 0 to be in cooldown")
	}
	until := cp.deactivationUntil(0)
	if until.IsZero() {
		t.Fatalf("expected non-zero cooldown until")
	}
	remaining := until.Sub(time.Now())
	if remaining > time.Hour+5*time.Second {
		t.Fatalf("expected cooldown capped near 1h, got %s", remaining)
	}
}

func TestForwardWithFailover_AllProvidersCooldownReturnsRetryAfter(t *testing.T) {
	t.Parallel()

	rt := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Host {
		case "p1":
			h := make(http.Header)
			h.Set("Content-Type", "application/json")
			h.Set("Retry-After", "30")
			return newResponse(http.StatusTooManyRequests, h, `{"error":{"message":"rate limit","type":"rate_limit_exceeded","code":"rate_limit_exceeded"}}`), nil
		case "p2":
			h := make(http.Header)
			h.Set("Content-Type", "application/json")
			h.Set("Retry-After", "45")
			return newResponse(http.StatusTooManyRequests, h, `{"type":"error","error":{"type":"rate_limit_error","message":"rate limit"}}`), nil
		default:
			return nil, errors.New("unexpected host")
		}
	})

	cp := newClientProxy(ClientCodex, []config.Provider{
		{Name: "p1", BaseURL: "http://p1", APIKey: "k1", Priority: 1},
		{Name: "p2", BaseURL: "http://p2", APIKey: "k2", Priority: 2},
	}, time.Hour)
	cp.httpClient.Transport = rt

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://proxy/codex/v1/test", bytes.NewReader([]byte(`{"x":1}`)))
	cp.forwardWithFailover(rr, req, "/v1/test")

	res := rr.Result()
	if res.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status: got %d want %d", res.StatusCode, http.StatusTooManyRequests)
	}
	if ra := res.Header.Get("Retry-After"); ra == "" {
		t.Fatalf("expected Retry-After header to be set")
	}
}
