package proxy

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lansespirit/Clipal/internal/config"
	oauthpkg "github.com/lansespirit/Clipal/internal/oauth"
)

func TestCreateProxyRequest_CodexOAuthNonStreamingUsesCompactEndpoint(t *testing.T) {
	dir := t.TempDir()
	svc := oauthpkg.NewService(dir)
	if err := svc.Store().Save(&oauthpkg.Credential{
		Ref:         "codex-sean-example-com",
		Provider:    config.OAuthProviderCodex,
		Email:       "sean@example.com",
		AccountID:   "acct_123",
		AccessToken: "access-1",
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	cp := newClientProxy(ClientOpenAI, config.ClientModeAuto, "", []config.Provider{
		{
			Name:          "codex-oauth",
			AuthType:      config.ProviderAuthTypeOAuth,
			OAuthProvider: config.OAuthProviderCodex,
			OAuthRef:      "codex-sean-example-com",
			Priority:      1,
			Overrides: &config.ProviderOverrides{
				Model: strPtr("gpt-5.4"),
				OpenAI: &config.OpenAIOverrides{
					ReasoningEffort: strPtr("high"),
				},
			},
		},
	}, time.Hour, 0, testResponseHeaderTimeout, circuitBreakerConfig{})
	cp.oauth = svc

	body := []byte(`{"model":"gpt-4.1","instructions":null,"stream":false,"store":true,"stream_options":{"include_usage":true},"input":"hello"}`)
	original := httptest.NewRequest(http.MethodPost, "http://proxy/clipal/v1/responses?foo=bar", bytes.NewReader(body))
	original.Header.Set("Content-Type", "application/json")
	original.Header.Set("Authorization", "Bearer client-key")
	original.Header.Set("Cookie", "secret=1")
	original.Header.Set("X-Test", "keep")
	original = withRequestContext(original, RequestContext{
		ClientType:     ClientOpenAI,
		Family:         ProtocolFamilyOpenAI,
		Capability:     CapabilityOpenAIResponses,
		UpstreamPath:   "/v1/responses",
		UnifiedIngress: true,
	})

	proxyReq, err := cp.createProxyRequest(original, cp.providers[0], "", "/v1/responses", body)
	if err != nil {
		t.Fatalf("createProxyRequest: %v", err)
	}
	if got := proxyReq.URL.String(); got != "https://chatgpt.com/backend-api/codex/responses/compact?foo=bar" {
		t.Fatalf("url = %q", got)
	}
	if got := proxyReq.Header.Get("Authorization"); got != "Bearer access-1" {
		t.Fatalf("Authorization = %q", got)
	}
	if got := proxyReq.Header.Get("Accept"); got != "application/json" {
		t.Fatalf("Accept = %q", got)
	}
	if got := proxyReq.Header.Get("Originator"); got != codexOAuthOriginator {
		t.Fatalf("Originator = %q", got)
	}
	if got := proxyReq.Header.Get("Chatgpt-Account-Id"); got != "acct_123" {
		t.Fatalf("Chatgpt-Account-Id = %q", got)
	}
	if got := proxyReq.Header.Get("Cookie"); got != "" {
		t.Fatalf("Cookie = %q, want empty", got)
	}
	if got := proxyReq.Header.Get("X-Test"); got != "" {
		t.Fatalf("X-Test = %q, want empty", got)
	}
	if got := proxyReq.Header.Get("Session_id"); got == "" {
		t.Fatalf("expected Session_id to be generated")
	}
	if got := proxyReq.Header.Get("Version"); got != codexOAuthVersion {
		t.Fatalf("Version = %q", got)
	}
	if got := proxyReq.Header.Get("OpenAI-Beta"); got != "responses=experimental" {
		t.Fatalf("OpenAI-Beta = %q", got)
	}

	root := decodeRequestBodyMap(t, proxyReq)
	if got := root["model"]; got != "gpt-5.4" {
		t.Fatalf("model = %v", got)
	}
	reasoning, ok := root["reasoning"].(map[string]any)
	if !ok {
		t.Fatalf("reasoning = %T %#v", root["reasoning"], root["reasoning"])
	}
	if got := reasoning["effort"]; got != "high" {
		t.Fatalf("reasoning.effort = %v", got)
	}
	if got := root["instructions"]; got != "" {
		t.Fatalf("instructions = %#v", got)
	}
	if _, ok := root["stream"]; ok {
		t.Fatalf("did not expect stream field in compact body: %#v", root)
	}
	if _, ok := root["store"]; ok {
		t.Fatalf("did not expect store field in compact body: %#v", root)
	}
	if _, ok := root["stream_options"]; ok {
		t.Fatalf("did not expect stream_options in compact body: %#v", root)
	}
	input, ok := root["input"].([]any)
	if !ok || len(input) != 1 {
		t.Fatalf("input = %#v", root["input"])
	}
	msg, ok := input[0].(map[string]any)
	if !ok {
		t.Fatalf("input[0] = %T %#v", input[0], input[0])
	}
	if got := msg["role"]; got != "user" {
		t.Fatalf("input[0].role = %v", got)
	}
}

func TestCreateCodexOAuthRequest_RefreshUsesProviderCustomProxy(t *testing.T) {
	now := time.Date(2026, 4, 18, 21, 0, 0, 0, time.UTC)
	var proxyHits int32
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&proxyHits, 1)
		if got := r.URL.Host; got != "auth.example" {
			t.Fatalf("proxied host = %q, want auth.example", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"access-2","refresh_token":"refresh-2","expires_in":3600}`)
	}))
	defer proxyServer.Close()

	svc := oauthpkg.NewService(t.TempDir(), oauthpkg.WithCodexClient(&oauthpkg.CodexClient{
		TokenURL: "http://auth.example/token",
		ClientID: "test-client",
		HTTPClient: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			t.Fatalf("refresh used default oauth HTTP client")
			return nil, nil
		})},
		Now: func() time.Time { return now },
	}))
	if err := svc.Store().Save(&oauthpkg.Credential{
		Ref:          "codex-sean-example-com",
		Provider:     config.OAuthProviderCodex,
		Email:        "sean@example.com",
		AccountID:    "acct_123",
		AccessToken:  "access-1",
		RefreshToken: "refresh-1",
		ExpiresAt:    now.Add(-time.Minute),
		LastRefresh:  now.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	provider := config.Provider{
		Name:          "codex-oauth",
		AuthType:      config.ProviderAuthTypeOAuth,
		OAuthProvider: config.OAuthProviderCodex,
		OAuthRef:      "codex-sean-example-com",
		ProxyMode:     config.ProviderProxyModeCustom,
		ProxyURL:      proxyServer.URL,
		Priority:      1,
	}
	cp := newClientProxy(ClientOpenAI, config.ClientModeAuto, "", []config.Provider{provider}, time.Hour, 0, testResponseHeaderTimeout, circuitBreakerConfig{})
	cp.oauth = svc
	body := []byte(`{"model":"gpt-5.2","input":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "http://proxy/codex/v1/responses", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withRequestContext(req, RequestContext{
		ClientType:     ClientOpenAI,
		Family:         ProtocolFamilyOpenAI,
		Capability:     CapabilityOpenAIResponses,
		UpstreamPath:   "/v1/responses",
		UnifiedIngress: true,
	})

	proxyReq, err := cp.createCodexOAuthRequestWithPayloadForProvider(req, provider, 0, "/v1/responses", newRequestPayload(body))
	if err != nil {
		t.Fatalf("createCodexOAuthRequestWithPayloadForProvider: %v", err)
	}
	if got := proxyReq.Header.Get("Authorization"); got != "Bearer access-2" {
		t.Fatalf("Authorization = %q, want refreshed token", got)
	}
	if got := atomic.LoadInt32(&proxyHits); got != 1 {
		t.Fatalf("proxy hits = %d, want 1", got)
	}
}

func TestOAuthHTTPClientForProvider_ClaudeDirectPreservesProviderClient(t *testing.T) {
	providerDirect := config.Provider{
		Name:          "claude-oauth",
		AuthType:      config.ProviderAuthTypeOAuth,
		OAuthProvider: config.OAuthProviderClaude,
		OAuthRef:      "claude-ref",
		ProxyMode:     config.ProviderProxyModeDirect,
		Priority:      1,
	}
	cp := newClientProxy(ClientClaude, config.ClientModeAuto, "", []config.Provider{providerDirect}, time.Hour, 0, testResponseHeaderTimeout, circuitBreakerConfig{})
	if got := cp.oauthHTTPClientForProvider(providerDirect, 0); got != nil {
		t.Fatalf("provider direct oauthHTTPClientForProvider = %T, want nil", got)
	}

	providerDefault := config.Provider{
		Name:          "claude-oauth",
		AuthType:      config.ProviderAuthTypeOAuth,
		OAuthProvider: config.OAuthProviderClaude,
		OAuthRef:      "claude-ref",
		Priority:      1,
	}
	cp = newClientProxyWithGlobalProxy(ClientClaude, config.ClientModeAuto, "", []config.Provider{providerDefault}, time.Hour, 0, testResponseHeaderTimeout, circuitBreakerConfig{}, config.GlobalUpstreamProxyModeDirect, "")
	if got := cp.oauthHTTPClientForProvider(providerDefault, 0); got != nil {
		t.Fatalf("global direct oauthHTTPClientForProvider = %T, want nil", got)
	}

	codexDirect := config.Provider{
		Name:          "codex-oauth",
		AuthType:      config.ProviderAuthTypeOAuth,
		OAuthProvider: config.OAuthProviderCodex,
		OAuthRef:      "codex-ref",
		ProxyMode:     config.ProviderProxyModeDirect,
		Priority:      1,
	}
	cp = newClientProxy(ClientOpenAI, config.ClientModeAuto, "", []config.Provider{codexDirect}, time.Hour, 0, testResponseHeaderTimeout, circuitBreakerConfig{})
	if got := cp.oauthHTTPClientForProvider(codexDirect, 0); got == nil {
		t.Fatalf("codex direct oauthHTTPClientForProvider = nil, want direct override client")
	}
}

func TestCreateProxyRequest_CodexOAuthStreamingUsesResponsesEndpoint(t *testing.T) {
	dir := t.TempDir()
	svc := oauthpkg.NewService(dir)
	if err := svc.Store().Save(&oauthpkg.Credential{
		Ref:         "codex-sean-example-com",
		Provider:    config.OAuthProviderCodex,
		Email:       "sean@example.com",
		AccountID:   "acct_123",
		AccessToken: "access-1",
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	cp := newClientProxy(ClientOpenAI, config.ClientModeAuto, "", []config.Provider{
		{
			Name:          "codex-oauth",
			AuthType:      config.ProviderAuthTypeOAuth,
			OAuthProvider: config.OAuthProviderCodex,
			OAuthRef:      "codex-sean-example-com",
			Priority:      1,
		},
	}, time.Hour, 0, testResponseHeaderTimeout, circuitBreakerConfig{})
	cp.oauth = svc

	body := []byte(`{"model":"gpt-4.1","stream":true,"input":"hello"}`)
	original := httptest.NewRequest(http.MethodPost, "http://proxy/clipal/v1/responses", bytes.NewReader(body))
	original.Header.Set("Content-Type", "application/json")
	original.Header.Set("Originator", "clipal-test")
	original.Header.Set("User-Agent", "clipal-test/1.0")
	original.Header.Set("X-Codex-Turn-Metadata", `{"turn_id":"turn-1"}`)
	original.Header.Set("X-Client-Request-Id", "req-123")
	original.Header.Set("Cookie", "secret=1")
	original.Header.Set("X-Test", "keep")
	original = withRequestContext(original, RequestContext{
		ClientType:     ClientOpenAI,
		Family:         ProtocolFamilyOpenAI,
		Capability:     CapabilityOpenAIResponses,
		UpstreamPath:   "/v1/responses",
		UnifiedIngress: true,
	})

	proxyReq, err := cp.createProxyRequest(original, cp.providers[0], "", "/v1/responses", body)
	if err != nil {
		t.Fatalf("createProxyRequest: %v", err)
	}
	if got := proxyReq.URL.String(); got != "https://chatgpt.com/backend-api/codex/responses" {
		t.Fatalf("url = %q", got)
	}
	if got := proxyReq.Header.Get("Accept"); got != "text/event-stream" {
		t.Fatalf("Accept = %q", got)
	}
	if got := proxyReq.Header.Get("Originator"); got != "clipal-test" {
		t.Fatalf("Originator = %q", got)
	}
	if got := proxyReq.Header.Get("User-Agent"); got != "clipal-test/1.0" {
		t.Fatalf("User-Agent = %q", got)
	}
	if got := proxyReq.Header.Get("X-Codex-Turn-Metadata"); got != `{"turn_id":"turn-1"}` {
		t.Fatalf("X-Codex-Turn-Metadata = %q", got)
	}
	if got := proxyReq.Header.Get("X-Client-Request-Id"); got != "req-123" {
		t.Fatalf("X-Client-Request-Id = %q", got)
	}
	if got := proxyReq.Header.Get("Cookie"); got != "" {
		t.Fatalf("Cookie = %q, want empty", got)
	}
	if got := proxyReq.Header.Get("X-Test"); got != "" {
		t.Fatalf("X-Test = %q, want empty", got)
	}
	if got := proxyReq.Header.Get("Session_id"); got != "" {
		t.Fatalf("Session_id = %q, want empty", got)
	}
	if got := proxyReq.Header.Get("OpenAI-Beta"); got != "responses=experimental" {
		t.Fatalf("OpenAI-Beta = %q", got)
	}

	root := decodeRequestBodyMap(t, proxyReq)
	if got := root["stream"]; got != true {
		t.Fatalf("stream = %v", got)
	}
}

func TestBuildCodexOAuthRequest_NormalizesResponsesBodyForCompatibility(t *testing.T) {
	body := []byte(`{
		"model":"gpt-5.2",
		"stream":true,
		"store":true,
		"functions":[{"name":"apply_patch","parameters":{"type":"object"}}],
		"function_call":{"name":"apply_patch"},
		"prompt_cache_retention":{"ttl":"1h"},
		"max_output_tokens":2048,
		"temperature":0.2,
		"input":[
			{"type":"message","role":"system","content":"be strict"},
			{"type":"message","role":"user","content":"hello"}
		]
	}`)

	targetPath, stream, rewritten, err := buildCodexOAuthRequest("/v1/responses", body)
	if err != nil {
		t.Fatalf("buildCodexOAuthRequest: %v", err)
	}
	if targetPath != "/responses" {
		t.Fatalf("targetPath = %q", targetPath)
	}
	if !stream {
		t.Fatalf("stream = false, want true")
	}

	var root map[string]any
	if err := json.Unmarshal(rewritten, &root); err != nil {
		t.Fatalf("json.Unmarshal: %v body=%s", err, string(rewritten))
	}
	if got := root["store"]; got != false {
		t.Fatalf("store = %v", got)
	}
	if got := root["instructions"]; got != "be strict" {
		t.Fatalf("instructions = %#v", got)
	}
	if _, ok := root["functions"]; ok {
		t.Fatalf("functions should be removed: %#v", root["functions"])
	}
	if _, ok := root["function_call"]; ok {
		t.Fatalf("function_call should be removed: %#v", root["function_call"])
	}
	if _, ok := root["prompt_cache_retention"]; ok {
		t.Fatalf("prompt_cache_retention should be removed: %#v", root["prompt_cache_retention"])
	}
	if _, ok := root["max_output_tokens"]; ok {
		t.Fatalf("max_output_tokens should be removed: %#v", root["max_output_tokens"])
	}
	if _, ok := root["temperature"]; ok {
		t.Fatalf("temperature should be removed: %#v", root["temperature"])
	}
	tools, ok := root["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tools = %#v", root["tools"])
	}
	toolChoice, ok := root["tool_choice"].(map[string]any)
	if !ok {
		t.Fatalf("tool_choice = %T %#v", root["tool_choice"], root["tool_choice"])
	}
	function, ok := toolChoice["function"].(map[string]any)
	if !ok || function["name"] != "apply_patch" {
		t.Fatalf("tool_choice.function = %#v", toolChoice["function"])
	}
	input, ok := root["input"].([]any)
	if !ok || len(input) != 1 {
		t.Fatalf("input = %#v", root["input"])
	}
	msg, ok := input[0].(map[string]any)
	if !ok || msg["role"] != "user" {
		t.Fatalf("input[0] = %#v", input[0])
	}
}

func TestCreateProxyRequest_CodexOAuthIgnoresProviderBaseURL(t *testing.T) {
	dir := t.TempDir()
	svc := oauthpkg.NewService(dir)
	if err := svc.Store().Save(&oauthpkg.Credential{
		Ref:         "codex-sean-example-com",
		Provider:    config.OAuthProviderCodex,
		Email:       "sean@example.com",
		AccountID:   "acct_123",
		AccessToken: "access-1",
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	cp := newClientProxy(ClientOpenAI, config.ClientModeAuto, "", []config.Provider{
		{
			Name:          "codex-oauth",
			BaseURL:       "https://should-not-be-used.example/codex",
			AuthType:      config.ProviderAuthTypeOAuth,
			OAuthProvider: config.OAuthProviderCodex,
			OAuthRef:      "codex-sean-example-com",
			Priority:      1,
		},
	}, time.Hour, 0, testResponseHeaderTimeout, circuitBreakerConfig{})
	cp.oauth = svc

	body := []byte(`{"model":"gpt-4.1","input":"hello"}`)
	original := httptest.NewRequest(http.MethodPost, "http://proxy/clipal/v1/responses", bytes.NewReader(body))
	original.Header.Set("Content-Type", "application/json")
	original = withRequestContext(original, RequestContext{
		ClientType:     ClientOpenAI,
		Family:         ProtocolFamilyOpenAI,
		Capability:     CapabilityOpenAIResponses,
		UpstreamPath:   "/v1/responses",
		UnifiedIngress: true,
	})

	proxyReq, err := cp.createProxyRequest(original, cp.providers[0], "", "/v1/responses", body)
	if err != nil {
		t.Fatalf("createProxyRequest: %v", err)
	}
	if got := proxyReq.URL.String(); got != "https://chatgpt.com/backend-api/codex/responses/compact" {
		t.Fatalf("url = %q", got)
	}
}

func TestCreateProxyRequest_CodexOAuthRefreshesExpiredCredential(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 4, 18, 21, 0, 0, 0, time.UTC)
	var refreshCalls int32

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if got := r.Form.Get("grant_type"); got != "refresh_token" {
			t.Fatalf("grant_type = %q", got)
		}
		atomic.AddInt32(&refreshCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, fmt.Sprintf(`{"access_token":"access-2","refresh_token":"refresh-2","id_token":"%s","expires_in":3600}`, testCodexJWT("sean@example.com", "acct_123")))
	}))
	defer tokenServer.Close()

	svc := oauthpkg.NewService(dir,
		oauthpkg.WithNowFunc(func() time.Time { return now }),
		oauthpkg.WithRefreshSkew(30*time.Second),
		oauthpkg.WithCodexClient(&oauthpkg.CodexClient{
			AuthURL:      "https://auth.openai.com/oauth/authorize",
			TokenURL:     tokenServer.URL,
			ClientID:     "test-client",
			CallbackHost: "127.0.0.1",
			CallbackPort: 0,
			CallbackPath: "/auth/callback",
			HTTPClient:   tokenServer.Client(),
			Now:          func() time.Time { return now },
		}),
	)
	if err := svc.Store().Save(&oauthpkg.Credential{
		Ref:          "codex-sean-example-com",
		Provider:     config.OAuthProviderCodex,
		Email:        "sean@example.com",
		AccountID:    "acct_123",
		AccessToken:  "access-1",
		RefreshToken: "refresh-1",
		ExpiresAt:    now.Add(5 * time.Second),
		LastRefresh:  now.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	cp := newClientProxy(ClientOpenAI, config.ClientModeAuto, "", []config.Provider{
		{
			Name:          "codex-oauth",
			AuthType:      config.ProviderAuthTypeOAuth,
			OAuthProvider: config.OAuthProviderCodex,
			OAuthRef:      "codex-sean-example-com",
			Priority:      1,
		},
	}, time.Hour, 0, testResponseHeaderTimeout, circuitBreakerConfig{})
	cp.oauth = svc

	body := []byte(`{"model":"gpt-4.1","input":"hello"}`)
	original := httptest.NewRequest(http.MethodPost, "http://proxy/clipal/v1/responses", bytes.NewReader(body))
	original.Header.Set("Content-Type", "application/json")
	original = withRequestContext(original, RequestContext{
		ClientType:     ClientOpenAI,
		Family:         ProtocolFamilyOpenAI,
		Capability:     CapabilityOpenAIResponses,
		UpstreamPath:   "/v1/responses",
		UnifiedIngress: true,
	})

	proxyReq, err := cp.createProxyRequest(original, cp.providers[0], "", "/v1/responses", body)
	if err != nil {
		t.Fatalf("createProxyRequest: %v", err)
	}
	if got := proxyReq.Header.Get("Authorization"); got != "Bearer access-2" {
		t.Fatalf("Authorization = %q", got)
	}
	if got := atomic.LoadInt32(&refreshCalls); got != 1 {
		t.Fatalf("refresh calls = %d, want 1", got)
	}

	loaded, err := svc.Load(config.OAuthProviderCodex, "codex-sean-example-com")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.AccessToken != "access-2" {
		t.Fatalf("stored access token = %q", loaded.AccessToken)
	}
	if loaded.RefreshToken != "refresh-2" {
		t.Fatalf("stored refresh token = %q", loaded.RefreshToken)
	}
}

func TestForwardWithFailover_CodexOAuthBuildFailureFallsBackToAPIKeyProvider(t *testing.T) {
	cp := newClientProxy(ClientOpenAI, config.ClientModeAuto, "", []config.Provider{
		{
			Name:          "codex-oauth",
			AuthType:      config.ProviderAuthTypeOAuth,
			OAuthProvider: config.OAuthProviderCodex,
			OAuthRef:      "missing-credential",
			Priority:      1,
		},
		{
			Name:     "openai-api",
			BaseURL:  "http://api.example",
			APIKey:   "provider-key",
			Priority: 2,
		},
	}, time.Hour, 0, testResponseHeaderTimeout, circuitBreakerConfig{})
	cp.oauth = oauthpkg.NewService(t.TempDir())

	var calls int32
	rt := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		if got := r.URL.String(); got != "http://api.example/v1/responses" {
			t.Fatalf("url = %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer provider-key" {
			t.Fatalf("Authorization = %q", got)
		}
		return newResponse(http.StatusOK, http.Header{"Content-Type": []string{"application/json"}}, `{"ok":true}`), nil
	})
	cp.httpClient.Transport = rt

	rr := httptest.NewRecorder()
	body := []byte(`{"model":"gpt-4.1","input":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "http://proxy/codex/v1/responses", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withRequestContext(req, RequestContext{
		ClientType:     ClientOpenAI,
		Family:         ProtocolFamilyOpenAI,
		Capability:     CapabilityOpenAIResponses,
		UpstreamPath:   "/v1/responses",
		UnifiedIngress: true,
	})

	cp.forwardWithFailover(rr, req, "/v1/responses")

	if got := rr.Result().StatusCode; got != http.StatusOK {
		t.Fatalf("status = %d body=%s", got, rr.Body.String())
	}
	if got := rr.Body.String(); got != `{"ok":true}` {
		t.Fatalf("body = %q", got)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("upstream calls = %d, want 1", got)
	}
}

func TestForwardWithFailover_CodexOAuthSkipsUnsupportedCapabilityInAutoMode(t *testing.T) {
	cp := newClientProxy(ClientOpenAI, config.ClientModeAuto, "", []config.Provider{
		{
			Name:          "codex-oauth",
			AuthType:      config.ProviderAuthTypeOAuth,
			OAuthProvider: config.OAuthProviderCodex,
			OAuthRef:      "codex-sean-example-com",
			Priority:      1,
		},
		{
			Name:     "openai-api",
			BaseURL:  "http://api.example",
			APIKey:   "provider-key",
			Priority: 2,
		},
	}, time.Hour, 0, testResponseHeaderTimeout, circuitBreakerConfig{})
	cp.oauth = oauthpkg.NewService(t.TempDir())

	var calls int32
	cp.httpClient.Transport = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		if got := r.URL.String(); got != "http://api.example/v1/chat/completions" {
			t.Fatalf("url = %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer provider-key" {
			t.Fatalf("Authorization = %q", got)
		}
		return newResponse(http.StatusOK, http.Header{"Content-Type": []string{"application/json"}}, `{"ok":true}`), nil
	})

	body := []byte(`{"model":"gpt-4.1","messages":[{"role":"user","content":"hello"}]}`)
	req := httptest.NewRequest(http.MethodPost, "http://proxy/codex/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withRequestContext(req, RequestContext{
		ClientType:     ClientOpenAI,
		Family:         ProtocolFamilyOpenAI,
		Capability:     CapabilityOpenAIChatCompletions,
		UpstreamPath:   "/v1/chat/completions",
		UnifiedIngress: true,
	})

	rr := httptest.NewRecorder()
	cp.forwardWithFailover(rr, req, "/v1/chat/completions")

	if got := rr.Result().StatusCode; got != http.StatusOK {
		t.Fatalf("status = %d body=%s", got, rr.Body.String())
	}
	if got := rr.Body.String(); got != `{"ok":true}` {
		t.Fatalf("body = %q", got)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("upstream calls = %d, want 1", got)
	}
}

func TestForwardManual_CodexOAuthRejectsUnsupportedCapability(t *testing.T) {
	cp := newClientProxy(ClientOpenAI, config.ClientModeManual, "codex-oauth", []config.Provider{
		{
			Name:          "codex-oauth",
			AuthType:      config.ProviderAuthTypeOAuth,
			OAuthProvider: config.OAuthProviderCodex,
			OAuthRef:      "codex-sean-example-com",
			Priority:      1,
		},
		{
			Name:     "openai-api",
			BaseURL:  "http://api.example",
			APIKey:   "provider-key",
			Priority: 2,
		},
	}, time.Hour, 0, testResponseHeaderTimeout, circuitBreakerConfig{})
	cp.oauth = oauthpkg.NewService(t.TempDir())

	var calls int32
	cp.httpClient.Transport = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		return newResponse(http.StatusOK, http.Header{"Content-Type": []string{"application/json"}}, `{"ok":true}`), nil
	})

	body := []byte(`{"model":"gpt-4.1","messages":[{"role":"user","content":"hello"}]}`)
	req := httptest.NewRequest(http.MethodPost, "http://proxy/codex/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withRequestContext(req, RequestContext{
		ClientType:     ClientOpenAI,
		Family:         ProtocolFamilyOpenAI,
		Capability:     CapabilityOpenAIChatCompletions,
		UpstreamPath:   "/v1/chat/completions",
		UnifiedIngress: true,
	})

	rr := httptest.NewRecorder()
	cp.forwardWithFailover(rr, req, "/v1/chat/completions")

	if got := rr.Result().StatusCode; got != http.StatusServiceUnavailable {
		t.Fatalf("status = %d body=%s", got, rr.Body.String())
	}
	if got := rr.Body.String(); got != "Pinned provider only supports OpenAI Responses requests.\n" {
		t.Fatalf("body = %q", got)
	}
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Fatalf("upstream calls = %d, want 0", got)
	}
}

func TestForwardWithFailover_CodexOAuthRefreshesAndRetriesOn401(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 4, 20, 8, 0, 0, 0, time.UTC)
	var refreshCalls int32
	var upstreamCalls int32

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if got := r.Form.Get("grant_type"); got != "refresh_token" {
			t.Fatalf("grant_type = %q", got)
		}
		atomic.AddInt32(&refreshCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, fmt.Sprintf(`{"access_token":"access-2","refresh_token":"refresh-2","id_token":"%s","expires_in":3600}`, testCodexJWT("sean@example.com", "acct_123")))
	}))
	defer tokenServer.Close()

	svc := oauthpkg.NewService(dir,
		oauthpkg.WithNowFunc(func() time.Time { return now }),
		oauthpkg.WithCodexClient(&oauthpkg.CodexClient{
			AuthURL:      "https://auth.openai.com/oauth/authorize",
			TokenURL:     tokenServer.URL,
			ClientID:     "test-client",
			CallbackHost: "127.0.0.1",
			CallbackPort: 0,
			CallbackPath: "/auth/callback",
			HTTPClient:   tokenServer.Client(),
			Now:          func() time.Time { return now },
		}),
	)
	if err := svc.Store().Save(&oauthpkg.Credential{
		Ref:          "codex-sean-example-com",
		Provider:     config.OAuthProviderCodex,
		Email:        "sean@example.com",
		AccountID:    "acct_123",
		AccessToken:  "access-1",
		RefreshToken: "refresh-1",
		ExpiresAt:    now.Add(time.Hour),
		LastRefresh:  now.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	cp := newClientProxy(ClientOpenAI, config.ClientModeAuto, "", []config.Provider{
		{
			Name:          "codex-oauth",
			AuthType:      config.ProviderAuthTypeOAuth,
			OAuthProvider: config.OAuthProviderCodex,
			OAuthRef:      "codex-sean-example-com",
			Priority:      1,
		},
	}, time.Hour, 0, testResponseHeaderTimeout, circuitBreakerConfig{})
	cp.oauth = svc
	cp.httpClient.Transport = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		call := atomic.AddInt32(&upstreamCalls, 1)
		switch call {
		case 1:
			if got := r.Header.Get("Authorization"); got != "Bearer access-1" {
				t.Fatalf("Authorization(first) = %q", got)
			}
			return newResponse(http.StatusUnauthorized, http.Header{"Content-Type": []string{"application/json"}}, `{"error":{"type":"authentication_error","code":"token_invalid","message":"expired"}}`), nil
		case 2:
			if got := r.Header.Get("Authorization"); got != "Bearer access-2" {
				t.Fatalf("Authorization(second) = %q", got)
			}
			return newResponse(http.StatusOK, http.Header{"Content-Type": []string{"application/json"}}, `{"ok":true}`), nil
		default:
			t.Fatalf("unexpected upstream call %d", call)
			return nil, nil
		}
	})

	rr := httptest.NewRecorder()
	body := []byte(`{"model":"gpt-5.2","input":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "http://proxy/codex/v1/responses", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withRequestContext(req, RequestContext{
		ClientType:     ClientOpenAI,
		Family:         ProtocolFamilyOpenAI,
		Capability:     CapabilityOpenAIResponses,
		UpstreamPath:   "/v1/responses",
		UnifiedIngress: true,
	})

	cp.forwardWithFailover(rr, req, "/v1/responses")

	if got := rr.Result().StatusCode; got != http.StatusOK {
		t.Fatalf("status = %d body=%s", got, rr.Body.String())
	}
	if got := rr.Body.String(); got != `{"ok":true}` {
		t.Fatalf("body = %q", got)
	}
	if got := atomic.LoadInt32(&refreshCalls); got != 1 {
		t.Fatalf("refresh calls = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&upstreamCalls); got != 2 {
		t.Fatalf("upstream calls = %d, want 2", got)
	}
	if cp.isDeactivated(0) {
		t.Fatalf("provider should remain active after successful retry")
	}
	if cp.isKeyDeactivated(0, 0) {
		t.Fatalf("provider key should remain active after successful retry")
	}
}

func TestForwardManual_CodexOAuthRefreshesAndRetriesOn401(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 4, 20, 8, 0, 0, 0, time.UTC)
	var refreshCalls int32
	var upstreamCalls int32

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if got := r.Form.Get("grant_type"); got != "refresh_token" {
			t.Fatalf("grant_type = %q", got)
		}
		atomic.AddInt32(&refreshCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, fmt.Sprintf(`{"access_token":"access-2","refresh_token":"refresh-2","id_token":"%s","expires_in":3600}`, testCodexJWT("sean@example.com", "acct_123")))
	}))
	defer tokenServer.Close()

	svc := oauthpkg.NewService(dir,
		oauthpkg.WithNowFunc(func() time.Time { return now }),
		oauthpkg.WithCodexClient(&oauthpkg.CodexClient{
			AuthURL:      "https://auth.openai.com/oauth/authorize",
			TokenURL:     tokenServer.URL,
			ClientID:     "test-client",
			CallbackHost: "127.0.0.1",
			CallbackPort: 0,
			CallbackPath: "/auth/callback",
			HTTPClient:   tokenServer.Client(),
			Now:          func() time.Time { return now },
		}),
	)
	if err := svc.Store().Save(&oauthpkg.Credential{
		Ref:          "codex-sean-example-com",
		Provider:     config.OAuthProviderCodex,
		Email:        "sean@example.com",
		AccountID:    "acct_123",
		AccessToken:  "access-1",
		RefreshToken: "refresh-1",
		ExpiresAt:    now.Add(time.Hour),
		LastRefresh:  now.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	cp := newClientProxy(ClientOpenAI, config.ClientModeManual, "codex-oauth", []config.Provider{
		{
			Name:          "codex-oauth",
			AuthType:      config.ProviderAuthTypeOAuth,
			OAuthProvider: config.OAuthProviderCodex,
			OAuthRef:      "codex-sean-example-com",
			Priority:      1,
		},
	}, time.Hour, 0, testResponseHeaderTimeout, circuitBreakerConfig{})
	cp.oauth = svc
	cp.httpClient.Transport = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		call := atomic.AddInt32(&upstreamCalls, 1)
		switch call {
		case 1:
			if got := r.Header.Get("Authorization"); got != "Bearer access-1" {
				t.Fatalf("Authorization(first) = %q", got)
			}
			return newResponse(http.StatusUnauthorized, http.Header{"Content-Type": []string{"application/json"}}, `{"error":{"type":"authentication_error","code":"token_invalid","message":"expired"}}`), nil
		case 2:
			if got := r.Header.Get("Authorization"); got != "Bearer access-2" {
				t.Fatalf("Authorization(second) = %q", got)
			}
			return newResponse(http.StatusOK, http.Header{"Content-Type": []string{"application/json"}}, `{"ok":true}`), nil
		default:
			t.Fatalf("unexpected upstream call %d", call)
			return nil, nil
		}
	})

	rr := httptest.NewRecorder()
	body := []byte(`{"model":"gpt-5.2","input":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "http://proxy/codex/v1/responses", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withRequestContext(req, RequestContext{
		ClientType:     ClientOpenAI,
		Family:         ProtocolFamilyOpenAI,
		Capability:     CapabilityOpenAIResponses,
		UpstreamPath:   "/v1/responses",
		UnifiedIngress: true,
	})

	cp.forwardManual(rr, req, "/v1/responses")

	if got := rr.Result().StatusCode; got != http.StatusOK {
		t.Fatalf("status = %d body=%s", got, rr.Body.String())
	}
	if got := rr.Body.String(); got != `{"ok":true}` {
		t.Fatalf("body = %q", got)
	}
	if got := atomic.LoadInt32(&refreshCalls); got != 1 {
		t.Fatalf("refresh calls = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&upstreamCalls); got != 2 {
		t.Fatalf("upstream calls = %d, want 2", got)
	}
}

func testCodexJWT(email string, accountID string) string {
	header := `{"alg":"none","typ":"JWT"}`
	payload := fmt.Sprintf(`{"email":"%s","sub":"sub_123","https://api.openai.com/auth":{"chatgpt_account_id":"%s"}}`, email, accountID)
	return base64.RawURLEncoding.EncodeToString([]byte(header)) + "." +
		base64.RawURLEncoding.EncodeToString([]byte(payload)) + "."
}
