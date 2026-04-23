package proxy

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/lansespirit/Clipal/internal/config"
	oauthpkg "github.com/lansespirit/Clipal/internal/oauth"
)

const (
	defaultCodexOAuthBaseURL = "https://chatgpt.com/backend-api/codex"
	codexOAuthVersion        = "0.118.0"
	codexOAuthUserAgent      = "codex-tui/0.118.0 (Mac OS 26.3.1; arm64) iTerm.app/3.6.9 (codex-tui; 0.118.0)"
	codexOAuthOriginator     = "codex-tui"
)

var codexOAuthAllowedHeaders = map[string]bool{
	"accept":                true,
	"accept-language":       true,
	"content-type":          true,
	"conversation-id":       true,
	"conversation_id":       true,
	"openai-beta":           true,
	"user-agent":            true,
	"originator":            true,
	"session-id":            true,
	"session_id":            true,
	"x-codex-turn-state":    true,
	"x-codex-turn-metadata": true,
	"x-client-request-id":   true,
	"x-request-id":          true,
}

func providerSupportsCapability(provider config.Provider, capability RequestCapability) bool {
	if !provider.UsesOAuth() {
		return true
	}

	switch provider.NormalizedOAuthProvider() {
	case config.OAuthProviderCodex:
		return capability == CapabilityOpenAIResponses
	case config.OAuthProviderClaude:
		return capability == CapabilityClaudeMessages || capability == CapabilityClaudeCountTokens
	case config.OAuthProviderGemini:
		return capability == CapabilityGeminiGenerateContent ||
			capability == CapabilityGeminiStreamGenerate ||
			capability == CapabilityGeminiCountTokens
	default:
		return false
	}
}

func supportedCapabilitySummary(provider config.Provider) string {
	if !provider.UsesOAuth() {
		return "all configured request types"
	}

	switch provider.NormalizedOAuthProvider() {
	case config.OAuthProviderCodex:
		return "OpenAI Responses requests"
	case config.OAuthProviderClaude:
		return "Claude messages and count_tokens requests"
	case config.OAuthProviderGemini:
		return "Gemini generateContent, streamGenerateContent, and countTokens requests"
	default:
		return "its configured request types"
	}
}

func (cp *ClientProxy) createOAuthProxyRequest(original *http.Request, provider config.Provider, path string, body []byte) (*http.Request, error) {
	if cp == nil || cp.oauth == nil {
		return nil, fmt.Errorf("oauth service is unavailable")
	}

	switch provider.NormalizedOAuthProvider() {
	case config.OAuthProviderCodex:
		return cp.createCodexOAuthRequest(original, provider, path, body)
	case config.OAuthProviderClaude:
		return cp.createClaudeOAuthRequest(original, provider, path, body)
	case config.OAuthProviderGemini:
		return cp.createGeminiOAuthRequest(original, provider, path, body)
	default:
		return nil, fmt.Errorf("unsupported oauth provider %q", provider.NormalizedOAuthProvider())
	}
}

func (cp *ClientProxy) createCodexOAuthRequest(original *http.Request, provider config.Provider, path string, body []byte) (*http.Request, error) {
	if original == nil {
		return nil, fmt.Errorf("original request is nil")
	}

	requestCtx, ok := requestContextFromRequest(original)
	if !ok {
		requestCtx = requestContextForClientPath(cp.clientType, path, false)
	}
	if requestCtx.Capability != CapabilityOpenAIResponses {
		return nil, fmt.Errorf("codex oauth only supports OpenAI responses requests")
	}

	cred, err := cp.oauth.RefreshIfNeeded(original.Context(), provider.NormalizedOAuthProvider(), provider.NormalizedOAuthRef())
	if err != nil {
		return nil, fmt.Errorf("load oauth credential: %w", err)
	}
	accessToken := ""
	if cred != nil {
		accessToken = strings.TrimSpace(cred.AccessToken)
	}
	if accessToken == "" {
		return nil, fmt.Errorf("oauth credential %q has no access token", provider.NormalizedOAuthRef())
	}

	body = applyProviderRequestOverrides(original, requestCtx, provider, body)
	targetPath, stream, requestBody, err := buildCodexOAuthRequest(path, body)
	if err != nil {
		return nil, err
	}
	targetURL, err := buildTargetURL(defaultCodexOAuthBaseURL, targetPath, original.URL.RawQuery)
	if err != nil {
		return nil, err
	}

	proxyReq, err := http.NewRequestWithContext(original.Context(), original.Method, targetURL, bytes.NewReader(requestBody))
	if err != nil {
		return nil, err
	}
	copyCodexOAuthHeaders(proxyReq.Header, original.Header)

	addForwardedHeaders(proxyReq, original)
	clearAuthCarriers(proxyReq)
	proxyReq.Header.Set("Authorization", "Bearer "+accessToken)
	applyCodexOAuthHeaders(proxyReq, cred, stream)
	proxyReq.ContentLength = int64(len(requestBody))
	proxyReq.Header.Del("Content-Length")
	return proxyReq, nil
}

func buildCodexOAuthRequest(path string, body []byte) (string, bool, []byte, error) {
	path = normalizeUpstreamPath(path)

	switch {
	case matchesExactPath(path, "/v1/responses/compact"):
		_, rewritten, err := normalizeCodexOAuthResponsesBody(body, true)
		if err != nil {
			return "", false, nil, err
		}
		return "/responses/compact", false, rewritten, nil
	case matchesExactPath(path, "/v1/responses"):
		stream, rewritten, err := normalizeCodexOAuthResponsesBody(body, false)
		if err != nil {
			return "", false, nil, err
		}
		if stream {
			return "/responses", true, rewritten, nil
		}
		return "/responses/compact", false, rewritten, nil
	case pathMatchesPrefix(path, "/v1/responses/"):
		return strings.TrimPrefix(path, "/v1"), false, body, nil
	default:
		return "", false, nil, fmt.Errorf("codex oauth does not support path %q", path)
	}
}

func normalizeCodexOAuthResponsesBody(body []byte, forceCompact bool) (bool, []byte, error) {
	if len(bytes.TrimSpace(body)) == 0 {
		return false, nil, fmt.Errorf("responses request body is required")
	}

	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		return false, nil, fmt.Errorf("responses request body must be valid json: %w", err)
	}

	stream, _ := root["stream"].(bool)
	if forceCompact {
		stream = false
	}
	if stream {
		root["stream"] = true
		root["store"] = false
	} else {
		delete(root, "stream")
		delete(root, "store")
	}

	delete(root, "prompt_cache_retention")
	delete(root, "safety_identifier")
	delete(root, "stream_options")
	delete(root, "max_output_tokens")
	delete(root, "max_completion_tokens")
	delete(root, "temperature")
	delete(root, "top_p")
	delete(root, "frequency_penalty")
	delete(root, "presence_penalty")
	normalizeCodexOAuthLegacyFunctionFields(root)
	normalizeCodexOAuthInput(root)
	if instructions, ok := root["instructions"]; !ok || instructions == nil {
		root["instructions"] = ""
	}

	rewritten, err := json.Marshal(root)
	if err != nil {
		return false, nil, fmt.Errorf("marshal codex oauth request: %w", err)
	}
	return stream, rewritten, nil
}

func applyCodexOAuthHeaders(proxyReq *http.Request, cred *oauthpkg.Credential, stream bool) {
	if proxyReq == nil {
		return
	}

	proxyReq.Header.Set("Content-Type", "application/json")
	if stream {
		proxyReq.Header.Set("Accept", "text/event-stream")
	} else {
		proxyReq.Header.Set("Accept", "application/json")
	}
	if strings.TrimSpace(proxyReq.Header.Get("OpenAI-Beta")) == "" {
		proxyReq.Header.Set("OpenAI-Beta", "responses=experimental")
	}
	proxyReq.Header.Set("Connection", "Keep-Alive")
	if strings.TrimSpace(proxyReq.Header.Get("User-Agent")) == "" {
		proxyReq.Header.Set("User-Agent", codexOAuthUserAgent)
	}
	if strings.TrimSpace(proxyReq.Header.Get("Originator")) == "" {
		proxyReq.Header.Set("Originator", codexOAuthOriginator)
	}
	if !stream && strings.TrimSpace(proxyReq.Header.Get("Version")) == "" {
		proxyReq.Header.Set("Version", codexOAuthVersion)
	}
	if !stream && strings.TrimSpace(proxyReq.Header.Get("Session_id")) == "" {
		proxyReq.Header.Set("Session_id", newCodexSessionID())
	}

	if cred != nil && strings.TrimSpace(cred.AccountID) != "" {
		proxyReq.Header.Set("Chatgpt-Account-Id", strings.TrimSpace(cred.AccountID))
	} else {
		proxyReq.Header.Del("Chatgpt-Account-Id")
	}
}

func newCodexSessionID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return ""
	}
	return hex.EncodeToString(raw[:])
}

func copyCodexOAuthHeaders(dst http.Header, src http.Header) {
	for key, values := range src {
		if isHopByHopHeader(key) || !codexOAuthAllowedHeaders[strings.ToLower(strings.TrimSpace(key))] {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func normalizeCodexOAuthLegacyFunctionFields(root map[string]any) {
	if root == nil {
		return
	}
	if functionsRaw, ok := root["functions"]; ok {
		if functions, ok := functionsRaw.([]any); ok {
			existing, _ := root["tools"].([]any)
			tools := make([]any, 0, len(existing)+len(functions))
			tools = append(tools, existing...)
			for _, function := range functions {
				tools = append(tools, map[string]any{
					"type":     "function",
					"function": function,
				})
			}
			root["tools"] = tools
		}
		delete(root, "functions")
	}
	if functionCall, ok := root["function_call"]; ok {
		switch v := functionCall.(type) {
		case string:
			trimmed := strings.TrimSpace(v)
			if trimmed != "" {
				root["tool_choice"] = trimmed
			}
		case map[string]any:
			if name, ok := v["name"].(string); ok && strings.TrimSpace(name) != "" {
				root["tool_choice"] = map[string]any{
					"type": "function",
					"function": map[string]any{
						"name": strings.TrimSpace(name),
					},
				}
			}
		}
		delete(root, "function_call")
	}
}

func normalizeCodexOAuthInput(root map[string]any) {
	if root == nil {
		return
	}
	switch input := root["input"].(type) {
	case string:
		trimmed := strings.TrimSpace(input)
		if trimmed == "" {
			root["input"] = []any{}
		} else {
			root["input"] = []any{
				map[string]any{
					"type":    "message",
					"role":    "user",
					"content": input,
				},
			}
		}
	case []any:
		extractCodexOAuthSystemInstructions(root, input)
	}
}

func extractCodexOAuthSystemInstructions(root map[string]any, input []any) {
	if root == nil || len(input) == 0 {
		return
	}

	var systemTexts []string
	filtered := make([]any, 0, len(input))
	for _, item := range input {
		msg, ok := item.(map[string]any)
		if !ok {
			filtered = append(filtered, item)
			continue
		}
		role, _ := msg["role"].(string)
		if !strings.EqualFold(strings.TrimSpace(role), "system") {
			filtered = append(filtered, item)
			continue
		}
		if text := extractCodexOAuthContentText(msg["content"]); text != "" {
			systemTexts = append(systemTexts, text)
		}
	}
	if len(systemTexts) == 0 {
		return
	}

	instructions := strings.Join(systemTexts, "\n\n")
	if existing, ok := root["instructions"].(string); ok && strings.TrimSpace(existing) != "" {
		root["instructions"] = instructions + "\n\n" + strings.TrimSpace(existing)
	} else {
		root["instructions"] = instructions
	}
	root["input"] = filtered
}

func extractCodexOAuthContentText(content any) string {
	switch v := content.(type) {
	case string:
		return strings.TrimSpace(v)
	case []any:
		parts := make([]string, 0, len(v))
		for _, part := range v {
			msg, ok := part.(map[string]any)
			if !ok {
				continue
			}
			partType, _ := msg["type"].(string)
			if partType != "" && partType != "text" && partType != "input_text" {
				continue
			}
			text, _ := msg["text"].(string)
			text = strings.TrimSpace(text)
			if text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "")
	default:
		return ""
	}
}

func (cp *ClientProxy) doProviderRequest(original *http.Request, provider config.Provider, providerIndex int, apiKey string, path string, body []byte) (*http.Response, bool, error) {
	proxyReq, err := cp.createProxyRequest(original, provider, apiKey, path, body)
	if err != nil {
		return nil, false, err
	}
	resp, err := cp.doPreparedProviderRequest(proxyReq, providerIndex, body)
	if err != nil || !provider.UsesOAuth() || resp == nil || resp.StatusCode != http.StatusUnauthorized {
		if err != nil || resp == nil {
			return resp, true, err
		}
		resp, err = prepareOAuthProviderResponse(original, provider, resp)
		return resp, true, err
	}
	if cp == nil || cp.oauth == nil {
		return resp, true, err
	}
	refreshed, refreshErr := cp.oauth.Refresh(original.Context(), provider.NormalizedOAuthProvider(), provider.NormalizedOAuthRef())
	if refreshErr != nil || refreshed == nil || strings.TrimSpace(refreshed.AccessToken) == "" {
		return resp, true, err
	}

	_ = resp.Body.Close()
	proxyReq, err = cp.createProxyRequest(original, provider, apiKey, path, body)
	if err != nil {
		return nil, false, err
	}
	resp, err = cp.doPreparedProviderRequest(proxyReq, providerIndex, body)
	if err != nil || resp == nil {
		return resp, true, err
	}
	resp, err = prepareOAuthProviderResponse(original, provider, resp)
	return resp, true, err
}

func (cp *ClientProxy) doPreparedProviderRequest(proxyReq *http.Request, providerIndex int, body []byte) (*http.Response, error) {
	if proxyReq == nil {
		return nil, fmt.Errorf("proxy request is nil")
	}
	proxyReq.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	//nolint:gosec // proxyReq.URL is controlled by buildCodexOAuthRequest, not user input
	return cp.upstreamHTTPClient(providerIndex).Do(proxyReq)
}
