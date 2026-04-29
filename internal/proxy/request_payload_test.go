package proxy

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lansespirit/Clipal/internal/config"
)

func TestRequestPayloadCachesCodexOAuthRewrite(t *testing.T) {
	body := []byte(`{"model":"gpt-5.2","stream":false,"input":"hello"}`)
	payload := newRequestPayload(body)
	req := httptest.NewRequest(http.MethodPost, "http://proxy/v1/responses", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	requestCtx := RequestContext{
		ClientType: ClientOpenAI,
		Family:     ProtocolFamilyOpenAI,
		Capability: CapabilityOpenAIResponses,
	}
	provider := config.Provider{
		Name: "codex",
	}

	_, _, first, err := payload.codexOAuthRequest(req, requestCtx, provider, "/v1/responses")
	if err != nil {
		t.Fatalf("codexOAuthRequest first: %v", err)
	}
	_, _, second, err := payload.codexOAuthRequest(req, requestCtx, provider, "/v1/responses")
	if err != nil {
		t.Fatalf("codexOAuthRequest second: %v", err)
	}
	if len(first) == 0 || len(second) == 0 || &first[0] != &second[0] {
		t.Fatalf("expected cached codex rewrite body to be reused")
	}
}

func TestRequestPayloadCachesGeminiOAuthRewrite(t *testing.T) {
	body := []byte(`{"contents":[{"role":"user","parts":[{"text":"hello"}]}]}`)
	payload := newRequestPayload(body)
	req := httptest.NewRequest(http.MethodPost, "http://proxy/v1beta/models/gemini-2.5-pro:generateContent", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	requestCtx := RequestContext{
		ClientType: ClientGemini,
		Family:     ProtocolFamilyGemini,
		Capability: CapabilityGeminiGenerateContent,
	}
	provider := config.Provider{Name: "gemini"}

	_, _, first, err := payload.geminiOAuthRequest(req, requestCtx, provider, "/v1beta/models/gemini-2.5-pro:generateContent", "project-1")
	if err != nil {
		t.Fatalf("geminiOAuthRequest first: %v", err)
	}
	_, _, second, err := payload.geminiOAuthRequest(req, requestCtx, provider, "/v1beta/models/gemini-2.5-pro:generateContent", "project-1")
	if err != nil {
		t.Fatalf("geminiOAuthRequest second: %v", err)
	}
	if len(first) == 0 || len(second) == 0 || &first[0] != &second[0] {
		t.Fatalf("expected cached gemini rewrite body to be reused")
	}
}
