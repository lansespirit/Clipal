package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lansespirit/Clipal/internal/config"
	oauthpkg "github.com/lansespirit/Clipal/internal/oauth"
)

func TestCreateProxyRequest_ClaudeOAuthUsesBearerAuth(t *testing.T) {
	dir := t.TempDir()
	svc := oauthpkg.NewService(dir)
	if err := svc.Store().Save(&oauthpkg.Credential{
		Ref:         "claude-sean-example-com",
		Provider:    config.OAuthProviderClaude,
		Email:       "sean@example.com",
		AccessToken: "access-1",
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	cp := newClientProxy(ClientClaude, config.ClientModeAuto, "", []config.Provider{
		{
			Name:          "claude-oauth",
			AuthType:      config.ProviderAuthTypeOAuth,
			OAuthProvider: config.OAuthProviderClaude,
			OAuthRef:      "claude-sean-example-com",
			Priority:      1,
			Overrides: &config.ProviderOverrides{
				Model: strPtr("claude-sonnet-4-5"),
				Claude: &config.ClaudeOverrides{
					ThinkingBudgetTokens: claudeTestIntPtr(2048),
				},
			},
		},
	}, time.Hour, 0, testResponseHeaderTimeout, circuitBreakerConfig{})
	cp.oauth = svc

	body := []byte(`{"model":"claude-3-7-sonnet","messages":[]}`)
	original := httptest.NewRequest(http.MethodPost, "http://proxy/clipal/v1/messages?beta=true", bytes.NewReader(body))
	original.Header.Set("Content-Type", "application/json")
	original.Header.Set("x-api-key", "client-key")
	original.Header.Set("anthropic-version", "2023-06-01")
	original.Header.Set("Cookie", "secret=1")
	original.Header.Set("Proxy-Authorization", "Basic dGVzdA==")
	original = withRequestContext(original, RequestContext{
		ClientType:     ClientClaude,
		Family:         ProtocolFamilyClaude,
		Capability:     CapabilityClaudeMessages,
		UpstreamPath:   "/v1/messages",
		UnifiedIngress: true,
	})

	proxyReq, err := cp.createProxyRequest(original, cp.providers[0], "", "/v1/messages", body)
	if err != nil {
		t.Fatalf("createProxyRequest: %v", err)
	}
	if got := proxyReq.URL.String(); got != "https://api.anthropic.com/v1/messages?beta=true" {
		t.Fatalf("url = %q", got)
	}
	if got := proxyReq.Header.Get("Authorization"); got != "Bearer access-1" {
		t.Fatalf("Authorization = %q", got)
	}
	if got := proxyReq.Header.Get("x-api-key"); got != "" {
		t.Fatalf("x-api-key = %q, want empty", got)
	}
	if got := proxyReq.Header.Get("anthropic-version"); got != "2023-06-01" {
		t.Fatalf("anthropic-version = %q", got)
	}
	if got := proxyReq.Header.Get("User-Agent"); got != claudeOAuthUserAgent {
		t.Fatalf("User-Agent = %q, want %q", got, claudeOAuthUserAgent)
	}
	if got := proxyReq.Header.Get("X-App"); got != "cli" {
		t.Fatalf("X-App = %q, want cli", got)
	}
	if got := proxyReq.Header.Get("Anthropic-Dangerous-Direct-Browser-Access"); got != "true" {
		t.Fatalf("Anthropic-Dangerous-Direct-Browser-Access = %q, want true", got)
	}
	if got := proxyReq.Header.Get("X-Stainless-Runtime"); got != claudeOAuthStainlessRuntime {
		t.Fatalf("X-Stainless-Runtime = %q, want %q", got, claudeOAuthStainlessRuntime)
	}
	if got := proxyReq.Header.Get("X-Stainless-Package-Version"); got != claudeOAuthStainlessPackageVersion {
		t.Fatalf("X-Stainless-Package-Version = %q, want %q", got, claudeOAuthStainlessPackageVersion)
	}
	betas := proxyReq.Header.Get("Anthropic-Beta")
	for _, token := range []string{"oauth-2025-04-20", "claude-code-20250219", "interleaved-thinking-2025-05-14", "prompt-caching-scope-2026-01-05", "effort-2025-11-24"} {
		if !strings.Contains(strings.ToLower(betas), strings.ToLower(token)) {
			t.Fatalf("Anthropic-Beta = %q, want token %q", betas, token)
		}
	}
	if got := proxyReq.Header.Get("Cookie"); got != "" {
		t.Fatalf("Cookie = %q, want empty", got)
	}
	if got := proxyReq.Header.Get("Proxy-Authorization"); got != "" {
		t.Fatalf("Proxy-Authorization = %q, want empty", got)
	}

	var root map[string]any
	if err := json.NewDecoder(proxyReq.Body).Decode(&root); err != nil {
		t.Fatalf("Decode body: %v", err)
	}
	if got := root["model"]; got != "claude-sonnet-4-5" {
		t.Fatalf("model = %v", got)
	}
	thinking, ok := root["thinking"].(map[string]any)
	if !ok {
		t.Fatalf("thinking = %#v", root["thinking"])
	}
	if got := thinking["budget_tokens"]; got != float64(2048) {
		t.Fatalf("thinking.budget_tokens = %v", got)
	}
}

func TestCreateProxyRequest_ClaudeOAuthResignsBillingHeaderAfterOverrides(t *testing.T) {
	dir := t.TempDir()
	svc := oauthpkg.NewService(dir)
	if err := svc.Store().Save(&oauthpkg.Credential{
		Ref:         "claude-sean-example-com",
		Provider:    config.OAuthProviderClaude,
		Email:       "sean@example.com",
		AccessToken: "access-1",
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	cp := newClientProxy(ClientClaude, config.ClientModeAuto, "", []config.Provider{
		{
			Name:          "claude-oauth",
			AuthType:      config.ProviderAuthTypeOAuth,
			OAuthProvider: config.OAuthProviderClaude,
			OAuthRef:      "claude-sean-example-com",
			Priority:      1,
			Overrides: &config.ProviderOverrides{
				Model: strPtr("claude-sonnet-4-5"),
			},
		},
	}, time.Hour, 0, testResponseHeaderTimeout, circuitBreakerConfig{})
	cp.oauth = svc

	body := []byte(`{
		"model":"claude-3-7-sonnet",
		"messages":[{"role":"user","content":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.81.a1b; cc_entrypoint=user; cch=abcde;"}]}],
		"metadata":{"note":"x-anthropic-billing-header: cc_version=2.1.81.a1b; cc_entrypoint=metadata; cch=fedcb;"},
		"system":[
			{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.81.a1b; cc_entrypoint=cli; cch=00000;"},
			{"type":"text","text":"You are Claude Code, Anthropic's official CLI for Claude."}
		]
	}`)
	original := httptest.NewRequest(http.MethodPost, "http://proxy/clipal/v1/messages", bytes.NewReader(body))
	original.Header.Set("Content-Type", "application/json")
	original = withRequestContext(original, RequestContext{
		ClientType:     ClientClaude,
		Family:         ProtocolFamilyClaude,
		Capability:     CapabilityClaudeMessages,
		UpstreamPath:   "/v1/messages",
		UnifiedIngress: true,
	})

	proxyReq, err := cp.createProxyRequest(original, cp.providers[0], "", "/v1/messages", body)
	if err != nil {
		t.Fatalf("createProxyRequest: %v", err)
	}

	rewrittenBody, err := io.ReadAll(proxyReq.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if strings.Contains(string(rewrittenBody), "cch=00000;") {
		t.Fatalf("body = %s, want billing header to be re-signed", string(rewrittenBody))
	}
	if got := signClaudeOAuthMessageBody(rewrittenBody); string(got) != string(rewrittenBody) {
		t.Fatalf("body was not emitted in signed form")
	}

	var root map[string]any
	if err := json.Unmarshal(rewrittenBody, &root); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if got := root["model"]; got != "claude-sonnet-4-5" {
		t.Fatalf("model = %v, want claude-sonnet-4-5", got)
	}
	system, ok := root["system"].([]any)
	if !ok || len(system) == 0 {
		t.Fatalf("system = %#v", root["system"])
	}
	systemBlock, ok := system[0].(map[string]any)
	if !ok {
		t.Fatalf("system[0] = %#v", system[0])
	}
	systemText, ok := systemBlock["text"].(string)
	if !ok {
		t.Fatalf("system[0].text = %#v", systemBlock["text"])
	}
	if strings.Contains(systemText, "cch=00000;") {
		t.Fatalf("system billing header was not re-signed: %q", systemText)
	}

	messages, ok := root["messages"].([]any)
	if !ok || len(messages) == 0 {
		t.Fatalf("messages = %#v", root["messages"])
	}
	message, ok := messages[0].(map[string]any)
	if !ok {
		t.Fatalf("messages[0] = %#v", messages[0])
	}
	content, ok := message["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatalf("messages[0].content = %#v", message["content"])
	}
	messageBlock, ok := content[0].(map[string]any)
	if !ok {
		t.Fatalf("messages[0].content[0] = %#v", content[0])
	}
	if got := messageBlock["text"]; got != "x-anthropic-billing-header: cc_version=2.1.81.a1b; cc_entrypoint=user; cch=abcde;" {
		t.Fatalf("messages[0].content[0].text = %#v", got)
	}

	metadata, ok := root["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("metadata = %#v", root["metadata"])
	}
	if got := metadata["note"]; got != "x-anthropic-billing-header: cc_version=2.1.81.a1b; cc_entrypoint=metadata; cch=fedcb;" {
		t.Fatalf("metadata.note = %#v", got)
	}
}

func TestForwardWithFailover_ClaudeOAuthRefreshesAndRetriesOn401(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)
	var refreshCalls int32
	var upstreamCalls int32

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q", got)
		}
		if got := string(body); got == "" {
			t.Fatalf("expected refresh body")
		}
		atomic.AddInt32(&refreshCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"access-2","refresh_token":"refresh-2","token_type":"Bearer","expires_in":3600,"account":{"uuid":"acct_123","email_address":"sean@example.com"},"organization":{"uuid":"org_123","name":"Example"}}`)
	}))
	defer tokenServer.Close()

	svc := oauthpkg.NewService(dir,
		oauthpkg.WithNowFunc(func() time.Time { return now }),
		oauthpkg.WithProviderClient(&oauthpkg.ClaudeClient{
			TokenURL:     tokenServer.URL,
			HTTPClient:   tokenServer.Client(),
			CallbackHost: "127.0.0.1",
			CallbackPort: 0,
			CallbackPath: "/callback",
			Now:          func() time.Time { return now },
		}),
	)
	if err := svc.Store().Save(&oauthpkg.Credential{
		Ref:          "claude-sean-example-com",
		Provider:     config.OAuthProviderClaude,
		Email:        "sean@example.com",
		AccessToken:  "access-1",
		RefreshToken: "refresh-1",
		ExpiresAt:    now.Add(time.Hour),
		LastRefresh:  now.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	cp := newClientProxy(ClientClaude, config.ClientModeAuto, "", []config.Provider{
		{
			Name:          "claude-oauth",
			AuthType:      config.ProviderAuthTypeOAuth,
			OAuthProvider: config.OAuthProviderClaude,
			OAuthRef:      "claude-sean-example-com",
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
			return newResponse(http.StatusUnauthorized, http.Header{"Content-Type": []string{"application/json"}}, `{"error":"expired"}`), nil
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
	body := []byte(`{"model":"claude-3-7-sonnet","messages":[]}`)
	req := httptest.NewRequest(http.MethodPost, "http://proxy/claudecode/v1/messages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withRequestContext(req, RequestContext{
		ClientType:     ClientClaude,
		Family:         ProtocolFamilyClaude,
		Capability:     CapabilityClaudeMessages,
		UpstreamPath:   "/v1/messages",
		UnifiedIngress: true,
	})

	cp.forwardWithFailover(rr, req, "/v1/messages")

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

func TestForwardCountTokensSingleShot_ClaudeOAuthRefreshesAndRetriesOn401(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)
	var refreshCalls int32
	var upstreamCalls int32

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q", got)
		}
		if got := string(body); got == "" {
			t.Fatalf("expected refresh body")
		}
		atomic.AddInt32(&refreshCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"access-2","refresh_token":"refresh-2","token_type":"Bearer","expires_in":3600,"account":{"uuid":"acct_123","email_address":"sean@example.com"},"organization":{"uuid":"org_123","name":"Example"}}`)
	}))
	defer tokenServer.Close()

	svc := oauthpkg.NewService(dir,
		oauthpkg.WithNowFunc(func() time.Time { return now }),
		oauthpkg.WithProviderClient(&oauthpkg.ClaudeClient{
			TokenURL:     tokenServer.URL,
			HTTPClient:   tokenServer.Client(),
			CallbackHost: "127.0.0.1",
			CallbackPort: 0,
			CallbackPath: "/callback",
			Now:          func() time.Time { return now },
		}),
	)
	if err := svc.Store().Save(&oauthpkg.Credential{
		Ref:          "claude-sean-example-com",
		Provider:     config.OAuthProviderClaude,
		Email:        "sean@example.com",
		AccessToken:  "access-1",
		RefreshToken: "refresh-1",
		ExpiresAt:    now.Add(time.Hour),
		LastRefresh:  now.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	cp := newClientProxy(ClientClaude, config.ClientModeAuto, "", []config.Provider{
		{
			Name:          "claude-oauth",
			AuthType:      config.ProviderAuthTypeOAuth,
			OAuthProvider: config.OAuthProviderClaude,
			OAuthRef:      "claude-sean-example-com",
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
			return newResponse(http.StatusUnauthorized, http.Header{"Content-Type": []string{"application/json"}}, `{"error":"expired"}`), nil
		case 2:
			if got := r.Header.Get("Authorization"); got != "Bearer access-2" {
				t.Fatalf("Authorization(second) = %q", got)
			}
			return newResponse(http.StatusOK, http.Header{"Content-Type": []string{"application/json"}}, `{"input_tokens":7}`), nil
		default:
			t.Fatalf("unexpected upstream call %d", call)
			return nil, nil
		}
	})

	rr := httptest.NewRecorder()
	body := []byte(`{"model":"claude-3-7-sonnet","messages":[]}`)
	req := httptest.NewRequest(http.MethodPost, "http://proxy/claudecode/v1/messages/count_tokens", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withRequestContext(req, RequestContext{
		ClientType:     ClientClaude,
		Family:         ProtocolFamilyClaude,
		Capability:     CapabilityClaudeCountTokens,
		UpstreamPath:   "/v1/messages/count_tokens",
		UnifiedIngress: true,
	})

	cp.forwardCountTokensSingleShot(rr, req, "/v1/messages/count_tokens")

	if got := rr.Result().StatusCode; got != http.StatusOK {
		t.Fatalf("status = %d body=%s", got, rr.Body.String())
	}
	if got := rr.Body.String(); got != `{"input_tokens":7}` {
		t.Fatalf("body = %q", got)
	}
	if got := atomic.LoadInt32(&refreshCalls); got != 1 {
		t.Fatalf("refresh calls = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&upstreamCalls); got != 2 {
		t.Fatalf("upstream calls = %d, want 2", got)
	}
}

func claudeTestIntPtr(v int) *int {
	return &v
}
