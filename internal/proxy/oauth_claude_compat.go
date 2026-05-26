package proxy

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"hash"
	"net/http"
	"regexp"
	"strings"

	xxhash "github.com/cespare/xxhash/v2"
)

const (
	claudeOAuthAnthropicVersion               = "2023-06-01"
	claudeOAuthAppVersion                     = "2.1.150"
	claudeOAuthUserAgent                      = "claude-cli/" + claudeOAuthAppVersion + " (external, cli)"
	claudeOAuthClientApp                      = "claude-code"
	claudeOAuthAppName                        = "claude-code"
	claudeOAuthXApp                           = "cli"
	claudeOAuthDangerousBrowserAccess         = "true"
	claudeOAuthStainlessRetryCount            = "0"
	claudeOAuthStainlessRuntime               = "node"
	claudeOAuthStainlessLang                  = "js"
	claudeOAuthStainlessTimeout               = "120"
	claudeOAuthStainlessPackageVersion        = claudeOAuthAppVersion
	claudeOAuthStainlessRuntimeVersion        = "v24.3.0"
	claudeOAuthStainlessOS                    = "MacOS"
	claudeOAuthStainlessArch                  = "arm64"
	claudeOAuthBillingHashSeed         uint64 = 0x6E52736AC806831E
)

var (
	claudeOAuthCLIUserAgentPattern = regexp.MustCompile(`(?i)^(claude-(?:cli|code))/([0-9]+(?:\.[0-9]+){1,3})\s+\(external,\s*(cli|sdk-cli)\)$`)
)

func normalizeClaudeOAuthRequest(body []byte, proxyReq *http.Request, original *http.Request, requestCtx RequestContext) []byte {
	body = ensureClaudeOAuthAttributionBlock(body, requestCtx)
	body = signClaudeOAuthMessageBody(body)
	applyClaudeOAuthHeaderDefaults(proxyReq, original, requestCtx)
	return body
}

func signClaudeOAuthMessageBody(body []byte) []byte {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return body
	}

	var root map[string]any
	if err := json.Unmarshal(trimmed, &root); err != nil {
		return body
	}

	unsignedRoot, ok := rewriteClaudeOAuthBillingHeaderPayloads(root, "00000")
	if !ok {
		return body
	}

	unsignedBody, err := json.Marshal(unsignedRoot)
	if err != nil {
		return body
	}
	signedCCH := claudeOAuthContentHash(unsignedBody)

	signedRoot, ok := rewriteClaudeOAuthBillingHeaderPayloads(root, signedCCH)
	if !ok {
		return body
	}
	signedBody, err := json.Marshal(signedRoot)
	if err != nil {
		return body
	}
	return signedBody
}

func claudeOAuthContentHash(payload []byte) string {
	var digest hash.Hash64 = xxhash.NewWithSeed(claudeOAuthBillingHashSeed)
	_, _ = digest.Write(payload)
	return fmt.Sprintf("%05x", digest.Sum64()&0xFFFFF)
}

func applyClaudeOAuthHeaderDefaults(proxyReq *http.Request, original *http.Request, requestCtx RequestContext) {
	if proxyReq == nil {
		return
	}

	inbound := http.Header(nil)
	if original != nil {
		inbound = original.Header
	}

	ensureClaudeOAuthHeader(proxyReq.Header, inbound, "Anthropic-Version", claudeOAuthAnthropicVersion)
	ensureClaudeOAuthHeader(proxyReq.Header, inbound, "X-App", claudeOAuthXApp)
	ensureClaudeOAuthHeader(proxyReq.Header, inbound, "X-App-Name", claudeOAuthAppName)
	ensureClaudeOAuthHeader(proxyReq.Header, inbound, "X-App-Ver", claudeOAuthAppVersion)
	ensureClaudeOAuthHeader(proxyReq.Header, inbound, "X-Client-App", claudeOAuthClientApp)

	if isOfficialClaudeCLIUserAgent(proxyReq.Header.Get("User-Agent")) {
		// Preserve official Claude Code fingerprints when they already exist.
	} else if isOfficialClaudeCLIUserAgent(headerValue(inbound, "User-Agent")) {
		proxyReq.Header.Set("User-Agent", headerValue(inbound, "User-Agent"))
	} else {
		proxyReq.Header.Set("User-Agent", claudeOAuthUserAgent)
	}

	requiredBetas := requiredClaudeOAuthBetas(requestCtx)
	if len(requiredBetas) > 0 {
		merged := mergeClaudeOAuthBetas(proxyReq.Header.Get("Anthropic-Beta"), requiredBetas)
		if strings.TrimSpace(merged) != "" {
			proxyReq.Header.Set("Anthropic-Beta", merged)
		}
	}

	if requestCtx.Capability == CapabilityClaudeMessages {
		ensureClaudeOAuthHeader(proxyReq.Header, inbound, "Anthropic-Dangerous-Direct-Browser-Access", claudeOAuthDangerousBrowserAccess)
		ensureClaudeOAuthHeader(proxyReq.Header, inbound, "Connection", "keep-alive")
		ensureClaudeOAuthHeader(proxyReq.Header, inbound, "X-Claude-Code-Session-Id", claudeOAuthSessionID())
		ensureClaudeOAuthHeader(proxyReq.Header, inbound, "X-Stainless-Retry-Count", claudeOAuthStainlessRetryCount)
		ensureClaudeOAuthHeader(proxyReq.Header, inbound, "X-Stainless-Runtime", claudeOAuthStainlessRuntime)
		ensureClaudeOAuthHeader(proxyReq.Header, inbound, "X-Stainless-Lang", claudeOAuthStainlessLang)
		ensureClaudeOAuthHeader(proxyReq.Header, inbound, "X-Stainless-Timeout", claudeOAuthStainlessTimeout)
		ensureClaudeOAuthHeader(proxyReq.Header, inbound, "X-Stainless-Package-Version", claudeOAuthStainlessPackageVersion)
		ensureClaudeOAuthHeader(proxyReq.Header, inbound, "X-Stainless-Runtime-Version", claudeOAuthStainlessRuntimeVersion)
		ensureClaudeOAuthHeader(proxyReq.Header, inbound, "X-Stainless-Os", claudeOAuthStainlessOS)
		ensureClaudeOAuthHeader(proxyReq.Header, inbound, "X-Stainless-Arch", claudeOAuthStainlessArch)
	} else {
		proxyReq.Header.Del("Anthropic-Dangerous-Direct-Browser-Access")
	}
}

func requiredClaudeOAuthBetas(requestCtx RequestContext) []string {
	betas := []string{
		"oauth-2025-04-20",
		"claude-code-20250219",
		"interleaved-thinking-2025-05-14",
		"prompt-caching-scope-2026-01-05",
		"effort-2025-11-24",
	}
	if requestCtx.Capability == CapabilityClaudeCountTokens {
		betas = append(betas, "token-counting-2024-11-01")
	}
	return betas
}

func mergeClaudeOAuthBetas(existing string, required []string) string {
	ordered := make([]string, 0, len(required)+4)
	seen := make(map[string]struct{}, len(required)+4)
	appendToken := func(token string) {
		token = strings.TrimSpace(token)
		if token == "" {
			return
		}
		lower := strings.ToLower(token)
		if _, ok := seen[lower]; ok {
			return
		}
		seen[lower] = struct{}{}
		ordered = append(ordered, token)
	}

	for _, token := range strings.Split(existing, ",") {
		appendToken(token)
	}
	for _, token := range required {
		appendToken(token)
	}
	return strings.Join(ordered, ",")
}

func ensureClaudeOAuthHeader(dst http.Header, inbound http.Header, key string, fallback string) {
	if dst == nil {
		return
	}
	if strings.TrimSpace(dst.Get(key)) != "" {
		return
	}
	if value := headerValue(inbound, key); value != "" {
		dst.Set(key, value)
		return
	}
	if strings.TrimSpace(fallback) != "" {
		dst.Set(key, fallback)
	}
}

func headerValue(header http.Header, key string) string {
	if header == nil {
		return ""
	}
	return strings.TrimSpace(header.Get(key))
}

func claudeOAuthSessionID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "clipal-" + claudeOAuthContentHash([]byte(claudeOAuthUserAgent))
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:16])
}

func isOfficialClaudeCLIUserAgent(value string) bool {
	return claudeOAuthCLIUserAgentPattern.MatchString(strings.TrimSpace(value))
}

func ensureClaudeOAuthAttributionBlock(body []byte, requestCtx RequestContext) []byte {
	if requestCtx.Capability != CapabilityClaudeMessages {
		return body
	}

	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return body
	}

	var root map[string]any
	if err := json.Unmarshal(trimmed, &root); err != nil {
		return body
	}
	if claudeOAuthHasBillingHeader(root["system"]) {
		return body
	}

	cloned, ok := cloneClaudeOAuthJSONValue(root).(map[string]any)
	if !ok || cloned == nil {
		return body
	}
	cloned["system"] = prependClaudeOAuthAttributionBlock(cloned["system"])

	rewritten, err := json.Marshal(cloned)
	if err != nil {
		return body
	}
	return rewritten
}

func prependClaudeOAuthAttributionBlock(existing any) any {
	attribution := map[string]any{
		"type": "text",
		"text": claudeOAuthBillingHeaderText("00000"),
	}

	switch typed := existing.(type) {
	case nil:
		return []any{attribution}
	case string:
		if strings.TrimSpace(typed) == "" {
			return []any{attribution}
		}
		return []any{
			attribution,
			map[string]any{"type": "text", "text": typed},
		}
	case []any:
		next := make([]any, 0, len(typed)+1)
		next = append(next, attribution)
		next = append(next, typed...)
		return next
	default:
		return []any{attribution, typed}
	}
}

func claudeOAuthBillingHeaderText(cch string) string {
	cch = strings.ToLower(strings.TrimSpace(cch))
	if cch == "" {
		cch = "00000"
	}
	return fmt.Sprintf("x-anthropic-billing-header: cc_version=%s; cc_entrypoint=cli; cc_workload=unknown; cch=%s;", claudeOAuthAppVersion, cch)
}

func claudeOAuthHasBillingHeader(value any) bool {
	switch typed := value.(type) {
	case string:
		return isClaudeOAuthBillingHeaderText(typed)
	case []any:
		for _, item := range typed {
			if claudeOAuthHasBillingHeader(item) {
				return true
			}
		}
		return false
	case map[string]any:
		if !strings.EqualFold(strings.TrimSpace(stringValue(typed["type"])), "text") {
			return false
		}
		text, _ := typed["text"].(string)
		return isClaudeOAuthBillingHeaderText(text)
	default:
		return false
	}
}

func isClaudeOAuthBillingHeaderText(text string) bool {
	trimmed := strings.TrimSpace(text)
	return strings.HasPrefix(strings.ToLower(trimmed), "x-anthropic-billing-header:")
}

func rewriteClaudeOAuthBillingHeaderPayloads(root map[string]any, cch string) (map[string]any, bool) {
	cloned, ok := cloneClaudeOAuthJSONValue(root).(map[string]any)
	if !ok || cloned == nil {
		return nil, false
	}

	rewritten := false
	for _, key := range []string{"system"} {
		value, exists := cloned[key]
		if !exists {
			continue
		}
		next, changed := rewriteClaudeOAuthBillingHeaderValue(value, cch)
		if changed {
			cloned[key] = next
			rewritten = true
		}
	}
	return cloned, rewritten
}

func rewriteClaudeOAuthBillingHeaderValue(value any, cch string) (any, bool) {
	switch typed := value.(type) {
	case string:
		return rewriteClaudeOAuthBillingHeaderText(typed, cch)
	case []any:
		rewritten := false
		for i, item := range typed {
			next, changed := rewriteClaudeOAuthBillingHeaderValue(item, cch)
			if changed {
				typed[i] = next
				rewritten = true
			}
		}
		return typed, rewritten
	case map[string]any:
		if !strings.EqualFold(strings.TrimSpace(stringValue(typed["type"])), "text") {
			return typed, false
		}
		text, ok := typed["text"].(string)
		if !ok {
			return typed, false
		}
		next, changed := rewriteClaudeOAuthBillingHeaderText(text, cch)
		if changed {
			typed["text"] = next
		}
		return typed, changed
	default:
		return value, false
	}
}

func rewriteClaudeOAuthBillingHeaderText(text string, cch string) (string, bool) {
	if !isClaudeOAuthBillingHeaderText(text) {
		return text, false
	}

	lower := strings.ToLower(text)
	valueStart := strings.Index(lower, "cch=")
	if valueStart < 0 {
		return text, false
	}
	valueStart += len("cch=")

	valueEnd := valueStart
	for valueEnd < len(text) && isClaudeOAuthHexDigit(text[valueEnd]) {
		valueEnd++
	}
	if valueEnd-valueStart != 5 || valueEnd >= len(text) || text[valueEnd] != ';' {
		return text, false
	}

	return text[:valueStart] + strings.ToLower(strings.TrimSpace(cch)) + text[valueEnd:], true
}

func isClaudeOAuthHexDigit(ch byte) bool {
	switch {
	case ch >= '0' && ch <= '9':
		return true
	case ch >= 'a' && ch <= 'f':
		return true
	case ch >= 'A' && ch <= 'F':
		return true
	default:
		return false
	}
}

func cloneClaudeOAuthJSONValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		cloned := make(map[string]any, len(typed))
		for key, item := range typed {
			cloned[key] = cloneClaudeOAuthJSONValue(item)
		}
		return cloned
	case []any:
		cloned := make([]any, len(typed))
		for i, item := range typed {
			cloned[i] = cloneClaudeOAuthJSONValue(item)
		}
		return cloned
	default:
		return value
	}
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}
