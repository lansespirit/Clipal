package proxy

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const maxRetryAfterCooldown = time.Hour
const shortBusyRetryAfterMax = 3 * time.Second

type failureAction int

const (
	failureReturnToClient failureAction = iota
	failureRetryNext
	failureBusyRetry
	failureDeactivateAndRetryNext
)

func classifyUpstreamFailure(status int, hdr http.Header, body []byte, truncated bool) (action failureAction, reason string, msg string, cooldown time.Duration) {
	snippet := sanitizeLogString(string(body))
	if truncated {
		snippet += "..."
	}
	snippet = truncateString(snippet, 2048)

	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return failureDeactivateAndRetryNext, "auth", snippet, 0
	case http.StatusPaymentRequired:
		return failureDeactivateAndRetryNext, "billing", snippet, 0
	case http.StatusTooManyRequests:
		cooldown = retryAfterDuration(hdr)
		action, reason := classify429(body)
		switch action {
		case failureDeactivateAndRetryNext:
			return action, reason, snippet, 0
		case failureBusyRetry:
			if cooldown > 0 && cooldown <= shortBusyRetryAfterMax {
				return action, reason, snippet, cooldown
			}
			return failureRetryNext, "rate_limit", snippet, cooldown
		case failureRetryNext:
			return action, reason, snippet, cooldown
		default:
			return failureRetryNext, "rate_limit", snippet, cooldown
		}
	default:
		if shouldRetry(status) {
			cooldown = retryAfterDuration(hdr)
			return failureRetryNext, "server", snippet, cooldown
		}
		return failureReturnToClient, "", "", 0
	}
}

func classify429(body []byte) (action failureAction, reason string) {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return failureRetryNext, "rate_limit"
	}

	code, typ, msg := extractErrorFields(v)

	code = strings.ToLower(code)
	typ = strings.ToLower(typ)
	msg = strings.ToLower(msg)

	// Hard failures: deactivate.
	if inSet(code, "invalid_api_key", "account_deactivated") ||
		inSet(typ, "authentication_error", "permission_error", "invalid_api_key") ||
		strings.Contains(msg, "invalid api key") {
		return failureDeactivateAndRetryNext, "auth"
	}
	if inSet(code, "insufficient_quota", "billing_hard_limit_reached", "organization_quota_exceeded") ||
		inSet(typ, "insufficient_quota", "billing_error") ||
		strings.Contains(msg, "insufficient quota") ||
		strings.Contains(msg, "billing") {
		return failureDeactivateAndRetryNext, "quota"
	}

	if hasConcurrencySignal(code, typ, msg) {
		return failureBusyRetry, "busy"
	}

	// Soft failures: retry on next provider.
	if inSet(code, "rate_limit_exceeded", "requests", "tokens") ||
		inSet(typ, "rate_limit_exceeded", "rate_limit_error", "overloaded_error") ||
		strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "too many requests") {
		if typ == "overloaded_error" {
			return failureRetryNext, "overloaded"
		}
		return failureRetryNext, "rate_limit"
	}

	// Default: treat as rate limit.
	return failureRetryNext, "rate_limit"
}

func hasConcurrencySignal(code string, typ string, msg string) bool {
	if strings.Contains(msg, "concurrent") ||
		strings.Contains(msg, "capacity") ||
		strings.Contains(msg, "too many active requests") {
		return true
	}
	if inSet(code, "concurrency_limit_exceeded") || inSet(typ, "concurrency_error") {
		return true
	}
	return false
}

func extractErrorFields(v any) (code string, typ string, msg string) {
	root, ok := v.(map[string]any)
	if !ok {
		return "", "", ""
	}

	// OpenAI-style: {"error": {"type": "...", "code": "...", "message": "..."}}.
	if errObj, ok := root["error"].(map[string]any); ok {
		if c, ok := errObj["code"].(string); ok {
			code = c
		}
		if t, ok := errObj["type"].(string); ok {
			typ = t
		}
		if m, ok := errObj["message"].(string); ok {
			msg = m
		}
		return code, typ, msg
	}

	// Anthropic-style: {"type":"error","error":{"type":"rate_limit_error","message":"..."}} already handled above.
	// Other providers: {"type":"error","code":"...","message":"..."}
	if c, ok := root["code"].(string); ok {
		code = c
	}
	if t, ok := root["type"].(string); ok {
		typ = t
	}
	if m, ok := root["message"].(string); ok {
		msg = m
	}
	return code, typ, msg
}

func inSet(v string, values ...string) bool {
	for _, s := range values {
		if v == s {
			return true
		}
	}
	return false
}

func retryAfterDuration(h http.Header) time.Duration {
	var max time.Duration
	if d, ok := parseRetryAfter(h.Get("Retry-After")); ok && d > max {
		max = d
	}
	// OpenAI-style hints (may be present even without Retry-After).
	if d, ok := parseDurationHint(h.Get("X-RateLimit-Reset-Requests")); ok && d > max {
		max = d
	}
	if d, ok := parseDurationHint(h.Get("X-RateLimit-Reset-Tokens")); ok && d > max {
		max = d
	}
	if max > maxRetryAfterCooldown {
		return maxRetryAfterCooldown
	}
	return max
}

func parseRetryAfter(v string) (time.Duration, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, false
	}
	// delta-seconds
	if secs, err := strconv.Atoi(v); err == nil {
		if secs < 0 {
			return 0, false
		}
		return time.Duration(secs) * time.Second, true
	}
	// HTTP-date
	if t, err := http.ParseTime(v); err == nil {
		d := time.Until(t)
		if d < 0 {
			return 0, true
		}
		return d, true
	}
	return 0, false
}

func parseDurationHint(v string) (time.Duration, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, false
	}
	// Common: "20ms" / "1s" / "2m".
	if d, err := time.ParseDuration(v); err == nil {
		if d < 0 {
			return 0, false
		}
		return d, true
	}
	// Sometimes: "12" meaning seconds.
	if secs, err := strconv.Atoi(v); err == nil {
		if secs < 0 {
			return 0, false
		}
		return time.Duration(secs) * time.Second, true
	}
	return 0, false
}

func shouldRetry(statusCode int) bool {
	switch statusCode {
	case http.StatusRequestTimeout,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
		529: // non-standard; used by some LLM providers for overloaded
		return true
	default:
		return false
	}
}
