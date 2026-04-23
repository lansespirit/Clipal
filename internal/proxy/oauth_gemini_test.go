package proxy

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lansespirit/Clipal/internal/config"
	oauthpkg "github.com/lansespirit/Clipal/internal/oauth"
)

type stubGeminiOAuthClient struct {
	refreshCred   *oauthpkg.Credential
	refreshErr    error
	refreshCalled int32
}

func (c *stubGeminiOAuthClient) Provider() config.OAuthProvider {
	return config.OAuthProviderGemini
}

func (c *stubGeminiOAuthClient) StartLogin(_ time.Time, _ time.Duration) (*oauthpkg.LoginSession, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *stubGeminiOAuthClient) ExchangeSessionCode(_ context.Context, _ *oauthpkg.LoginSession, _ string) (*oauthpkg.Credential, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *stubGeminiOAuthClient) Refresh(_ context.Context, cred *oauthpkg.Credential) (*oauthpkg.Credential, error) {
	atomic.AddInt32(&c.refreshCalled, 1)
	if c.refreshErr != nil {
		return nil, c.refreshErr
	}
	if c.refreshCred != nil {
		return c.refreshCred.Clone(), nil
	}
	return cred.Clone(), nil
}

func TestCreateProxyRequest_GeminiOAuthUsesBearerAuthAndProjectMetadata(t *testing.T) {
	dir := t.TempDir()
	svc := oauthpkg.NewService(dir)
	if err := svc.Store().Save(&oauthpkg.Credential{
		Ref:         "gemini-sean-example-com-gen-lang-client-123",
		Provider:    config.OAuthProviderGemini,
		Email:       "sean@example.com",
		AccessToken: "access-1",
		Metadata: map[string]string{
			"project_id": "gen-lang-client-123",
		},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	cp := newClientProxy(ClientGemini, config.ClientModeAuto, "", []config.Provider{
		{
			Name:          "gemini-oauth",
			AuthType:      config.ProviderAuthTypeOAuth,
			OAuthProvider: config.OAuthProviderGemini,
			OAuthRef:      "gemini-sean-example-com-gen-lang-client-123",
			Priority:      1,
		},
	}, time.Hour, 0, testResponseHeaderTimeout, circuitBreakerConfig{})
	cp.oauth = svc

	body := []byte(`{"contents":[{"parts":[{"text":"hello"}]}],"generationConfig":{"temperature":0.4},"user_prompt_id":"prompt-123","customField":{"keep":"me"}}`)
	original := httptest.NewRequest(http.MethodPost, "http://proxy/clipal/v1beta/models/gemini-2.5-flash:generateContent", bytes.NewReader(body))
	original.Header.Set("Content-Type", "application/json")
	original.Header.Set("x-goog-api-key", "client-key")
	original = withRequestContext(original, RequestContext{
		ClientType:     ClientGemini,
		Family:         ProtocolFamilyGemini,
		Capability:     CapabilityGeminiGenerateContent,
		UpstreamPath:   "/v1beta/models/gemini-2.5-flash:generateContent",
		UnifiedIngress: true,
	})

	proxyReq, err := cp.createProxyRequest(original, cp.providers[0], "", "/v1beta/models/gemini-2.5-flash:generateContent", body)
	if err != nil {
		t.Fatalf("createProxyRequest: %v", err)
	}
	if got := proxyReq.URL.String(); got != "https://cloudcode-pa.googleapis.com/v1internal:generateContent" {
		t.Fatalf("url = %q", got)
	}
	if got := proxyReq.Header.Get("Authorization"); got != "Bearer access-1" {
		t.Fatalf("Authorization = %q", got)
	}
	if got := proxyReq.Header.Get("x-goog-api-key"); got != "" {
		t.Fatalf("x-goog-api-key = %q, want empty", got)
	}
	if got := proxyReq.Header.Get("X-Goog-Api-Client"); got != geminiOAuthAPIClientHeader {
		t.Fatalf("X-Goog-Api-Client = %q", got)
	}
	if got := proxyReq.Header.Get("Accept"); got != "application/json" {
		t.Fatalf("Accept = %q", got)
	}

	root := decodeRequestBodyMap(t, proxyReq)
	if got := root["model"]; got != "gemini-2.5-flash" {
		t.Fatalf("model = %v", got)
	}
	if got := root["project"]; got != "gen-lang-client-123" {
		t.Fatalf("project = %v", got)
	}
	if got := root["user_prompt_id"]; got != "prompt-123" {
		t.Fatalf("user_prompt_id = %v", got)
	}
	request, ok := root["request"].(map[string]any)
	if !ok {
		t.Fatalf("request = %#v, want object", root["request"])
	}
	if _, ok := request["model"]; ok {
		t.Fatalf("did not expect nested model in request: %#v", request)
	}
	if _, ok := request["project"]; ok {
		t.Fatalf("did not expect nested project in request: %#v", request)
	}
	if _, ok := request["user_prompt_id"]; ok {
		t.Fatalf("did not expect nested user_prompt_id in request: %#v", request)
	}
	if _, ok := request["contents"]; !ok {
		t.Fatalf("request.contents missing: %#v", request)
	}
	if _, ok := request["generationConfig"]; !ok {
		t.Fatalf("request.generationConfig missing: %#v", request)
	}
	if got := request["customField"]; got == nil {
		t.Fatalf("request.customField missing: %#v", request)
	}
}

func TestCreateProxyRequest_GeminiOAuthStreamDefaultsAltSSE(t *testing.T) {
	dir := t.TempDir()
	svc := oauthpkg.NewService(dir)
	if err := svc.Store().Save(&oauthpkg.Credential{
		Ref:         "gemini-sean-example-com-gen-lang-client-123",
		Provider:    config.OAuthProviderGemini,
		Email:       "sean@example.com",
		AccessToken: "access-1",
		Metadata: map[string]string{
			"project_id": "gen-lang-client-123",
		},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	cp := newClientProxy(ClientGemini, config.ClientModeAuto, "", []config.Provider{
		{
			Name:          "gemini-oauth",
			AuthType:      config.ProviderAuthTypeOAuth,
			OAuthProvider: config.OAuthProviderGemini,
			OAuthRef:      "gemini-sean-example-com-gen-lang-client-123",
			Priority:      1,
		},
	}, time.Hour, 0, testResponseHeaderTimeout, circuitBreakerConfig{})
	cp.oauth = svc

	body := []byte(`{"contents":[]}`)
	original := httptest.NewRequest(http.MethodPost, "http://proxy/clipal/v1beta/models/gemini-2.5-pro:streamGenerateContent", bytes.NewReader(body))
	original = withRequestContext(original, RequestContext{
		ClientType:     ClientGemini,
		Family:         ProtocolFamilyGemini,
		Capability:     CapabilityGeminiStreamGenerate,
		UpstreamPath:   "/v1beta/models/gemini-2.5-pro:streamGenerateContent",
		UnifiedIngress: true,
	})

	proxyReq, err := cp.createProxyRequest(original, cp.providers[0], "", "/v1beta/models/gemini-2.5-pro:streamGenerateContent", body)
	if err != nil {
		t.Fatalf("createProxyRequest: %v", err)
	}
	if got := proxyReq.URL.String(); got != "https://cloudcode-pa.googleapis.com/v1internal:streamGenerateContent?alt=sse" {
		t.Fatalf("url = %q", got)
	}
	if got := proxyReq.Header.Get("Accept"); got != "text/event-stream" {
		t.Fatalf("Accept = %q", got)
	}

	root := decodeRequestBodyMap(t, proxyReq)
	if got := root["model"]; got != "gemini-2.5-pro" {
		t.Fatalf("model = %v", got)
	}
	if got := root["project"]; got != "gen-lang-client-123" {
		t.Fatalf("project = %v", got)
	}
	request, ok := root["request"].(map[string]any)
	if !ok {
		t.Fatalf("request = %#v, want object", root["request"])
	}
	if _, ok := request["contents"]; !ok {
		t.Fatalf("request.contents missing: %#v", request)
	}
}

func TestCreateProxyRequest_GeminiOAuthCountTokensPreservesRequestFields(t *testing.T) {
	dir := t.TempDir()
	svc := oauthpkg.NewService(dir)
	if err := svc.Store().Save(&oauthpkg.Credential{
		Ref:         "gemini-sean-example-com-gen-lang-client-123",
		Provider:    config.OAuthProviderGemini,
		Email:       "sean@example.com",
		AccessToken: "access-1",
		Metadata: map[string]string{
			"project_id": "gen-lang-client-123",
		},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	cp := newClientProxy(ClientGemini, config.ClientModeAuto, "", []config.Provider{
		{
			Name:          "gemini-oauth",
			AuthType:      config.ProviderAuthTypeOAuth,
			OAuthProvider: config.OAuthProviderGemini,
			OAuthRef:      "gemini-sean-example-com-gen-lang-client-123",
			Priority:      1,
		},
	}, time.Hour, 0, testResponseHeaderTimeout, circuitBreakerConfig{})
	cp.oauth = svc

	body := []byte(`{
		"model":"gemini-1.5-pro",
		"contents":[{"parts":[{"text":"count me"}]}],
		"generationConfig":{"temperature":0.2},
		"systemInstruction":{"parts":[{"text":"system"}]},
		"tools":[{"googleSearch":{}}],
		"cachedContent":"cachedContents/123"
	}`)
	original := httptest.NewRequest(http.MethodPost, "http://proxy/clipal/v1beta/models/gemini-2.5-pro:countTokens", bytes.NewReader(body))
	original = withRequestContext(original, RequestContext{
		ClientType:     ClientGemini,
		Family:         ProtocolFamilyGemini,
		Capability:     CapabilityGeminiCountTokens,
		UpstreamPath:   "/v1beta/models/gemini-2.5-pro:countTokens",
		UnifiedIngress: true,
	})

	proxyReq, err := cp.createProxyRequest(original, cp.providers[0], "", "/v1beta/models/gemini-2.5-pro:countTokens", body)
	if err != nil {
		t.Fatalf("createProxyRequest: %v", err)
	}
	if got := proxyReq.URL.String(); got != "https://cloudcode-pa.googleapis.com/v1internal:countTokens" {
		t.Fatalf("url = %q", got)
	}

	root := decodeRequestBodyMap(t, proxyReq)
	if _, ok := root["model"]; ok {
		t.Fatalf("did not expect top-level model in body: %#v", root)
	}
	request, ok := root["request"].(map[string]any)
	if !ok {
		t.Fatalf("request = %#v, want object", root["request"])
	}
	if got := request["model"]; got != "models/gemini-2.5-pro" {
		t.Fatalf("request.model = %v", got)
	}
	if _, ok := request["contents"]; !ok {
		t.Fatalf("request.contents missing: %#v", request)
	}
	if _, ok := request["generationConfig"]; !ok {
		t.Fatalf("request.generationConfig missing: %#v", request)
	}
	if _, ok := request["systemInstruction"]; !ok {
		t.Fatalf("request.systemInstruction missing: %#v", request)
	}
	if _, ok := request["tools"]; !ok {
		t.Fatalf("request.tools missing: %#v", request)
	}
	if got := request["cachedContent"]; got != "cachedContents/123" {
		t.Fatalf("request.cachedContent = %v", got)
	}
}

func TestForwardWithFailover_GeminiOAuthRefreshesAndRetriesOn401(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)
	client := &stubGeminiOAuthClient{
		refreshCred: &oauthpkg.Credential{
			Ref:          "gemini-sean-example-com-gen-lang-client-123",
			Provider:     config.OAuthProviderGemini,
			Email:        "sean@example.com",
			AccessToken:  "access-2",
			RefreshToken: "refresh-2",
			ExpiresAt:    now.Add(time.Hour),
			LastRefresh:  now,
			Metadata: map[string]string{
				"project_id": "gen-lang-client-123",
			},
		},
	}
	svc := oauthpkg.NewService(dir,
		oauthpkg.WithNowFunc(func() time.Time { return now }),
		oauthpkg.WithProviderClient(client),
	)
	if err := svc.Store().Save(&oauthpkg.Credential{
		Ref:          "gemini-sean-example-com-gen-lang-client-123",
		Provider:     config.OAuthProviderGemini,
		Email:        "sean@example.com",
		AccessToken:  "access-1",
		RefreshToken: "refresh-1",
		ExpiresAt:    now.Add(time.Hour),
		LastRefresh:  now.Add(-time.Hour),
		Metadata: map[string]string{
			"project_id": "gen-lang-client-123",
		},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	cp := newClientProxy(ClientGemini, config.ClientModeAuto, "", []config.Provider{
		{
			Name:          "gemini-oauth",
			AuthType:      config.ProviderAuthTypeOAuth,
			OAuthProvider: config.OAuthProviderGemini,
			OAuthRef:      "gemini-sean-example-com-gen-lang-client-123",
			Priority:      1,
		},
	}, time.Hour, 0, testResponseHeaderTimeout, circuitBreakerConfig{})
	cp.oauth = svc

	var upstreamCalls int32
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
	body := []byte(`{"contents":[]}`)
	req := httptest.NewRequest(http.MethodPost, "http://proxy/gemini/v1beta/models/gemini-2.5-pro:generateContent", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withRequestContext(req, RequestContext{
		ClientType:     ClientGemini,
		Family:         ProtocolFamilyGemini,
		Capability:     CapabilityGeminiGenerateContent,
		UpstreamPath:   "/v1beta/models/gemini-2.5-pro:generateContent",
		UnifiedIngress: true,
	})

	cp.forwardWithFailover(rr, req, "/v1beta/models/gemini-2.5-pro:generateContent")

	if got := rr.Result().StatusCode; got != http.StatusOK {
		t.Fatalf("status = %d body=%s", got, rr.Body.String())
	}
	if got := rr.Body.String(); got != `{"ok":true}` {
		t.Fatalf("body = %q", got)
	}
	if got := atomic.LoadInt32(&client.refreshCalled); got != 1 {
		t.Fatalf("refresh calls = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&upstreamCalls); got != 2 {
		t.Fatalf("upstream calls = %d, want 2", got)
	}
}

func TestForwardCountTokensSingleShot_GeminiOAuthRefreshesAndRetriesOn401(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)
	client := &stubGeminiOAuthClient{
		refreshCred: &oauthpkg.Credential{
			Ref:          "gemini-sean-example-com-gen-lang-client-123",
			Provider:     config.OAuthProviderGemini,
			Email:        "sean@example.com",
			AccessToken:  "access-2",
			RefreshToken: "refresh-2",
			ExpiresAt:    now.Add(time.Hour),
			LastRefresh:  now,
			Metadata: map[string]string{
				"project_id": "gen-lang-client-123",
			},
		},
	}
	svc := oauthpkg.NewService(dir,
		oauthpkg.WithNowFunc(func() time.Time { return now }),
		oauthpkg.WithProviderClient(client),
	)
	if err := svc.Store().Save(&oauthpkg.Credential{
		Ref:          "gemini-sean-example-com-gen-lang-client-123",
		Provider:     config.OAuthProviderGemini,
		Email:        "sean@example.com",
		AccessToken:  "access-1",
		RefreshToken: "refresh-1",
		ExpiresAt:    now.Add(time.Hour),
		LastRefresh:  now,
		Metadata: map[string]string{
			"project_id": "gen-lang-client-123",
		},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	cp := newClientProxy(ClientGemini, config.ClientModeAuto, "", []config.Provider{
		{
			Name:          "gemini-oauth",
			AuthType:      config.ProviderAuthTypeOAuth,
			OAuthProvider: config.OAuthProviderGemini,
			OAuthRef:      "gemini-sean-example-com-gen-lang-client-123",
			Priority:      1,
		},
	}, time.Hour, 0, testResponseHeaderTimeout, circuitBreakerConfig{})
	cp.oauth = svc

	var upstreamCalls int32
	cp.httpClient.Transport = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		switch got := atomic.AddInt32(&upstreamCalls, 1); got {
		case 1:
			if got := r.Header.Get("Authorization"); got != "Bearer access-1" {
				t.Fatalf("Authorization(first) = %q", got)
			}
			return newResponse(http.StatusUnauthorized, http.Header{"Content-Type": []string{"application/json"}}, `{"error":"expired"}`), nil
		case 2:
			if got := r.Header.Get("Authorization"); got != "Bearer access-2" {
				t.Fatalf("Authorization(second) = %q", got)
			}
			return newResponse(http.StatusOK, http.Header{"Content-Type": []string{"application/json"}}, `{"totalTokens":7}`), nil
		default:
			t.Fatalf("unexpected upstream call %d", got)
			return nil, nil
		}
	})

	rr := httptest.NewRecorder()
	body := []byte(`{"contents":[]}`)
	req := httptest.NewRequest(http.MethodPost, "http://proxy/gemini/v1beta/models/gemini-2.5-pro:countTokens", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withRequestContext(req, RequestContext{
		ClientType:     ClientGemini,
		Family:         ProtocolFamilyGemini,
		Capability:     CapabilityGeminiCountTokens,
		UpstreamPath:   "/v1beta/models/gemini-2.5-pro:countTokens",
		UnifiedIngress: true,
	})

	cp.forwardCountTokensSingleShot(rr, req, "/v1beta/models/gemini-2.5-pro:countTokens")

	if got := rr.Result().StatusCode; got != http.StatusOK {
		t.Fatalf("status = %d body=%s", got, rr.Body.String())
	}
	if got := rr.Body.String(); got != `{"totalTokens":7}` {
		t.Fatalf("body = %q", got)
	}
	if got := atomic.LoadInt32(&client.refreshCalled); got != 1 {
		t.Fatalf("refresh calls = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&upstreamCalls); got != 2 {
		t.Fatalf("upstream calls = %d, want 2", got)
	}
}
