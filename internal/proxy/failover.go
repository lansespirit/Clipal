package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lansespirit/Clipal/internal/logger"
)

const maxRetryAfterCooldown = time.Hour

type providerDeactivation struct {
	at      time.Time
	until   time.Time
	reason  string
	status  int
	message string
}

// forwardWithFailover forwards the request with automatic failover.
func (cp *ClientProxy) forwardWithFailover(w http.ResponseWriter, req *http.Request, path string) {
	cp.reactivateExpired()

	// Read the request body once for potential retries
	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		logger.Error("[%s] failed to read request body: %v", cp.clientType, err)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

	active := cp.activeProviderCount()
	if active == 0 {
		wait, reason, ok := cp.timeUntilNextAvailable()
		if ok && wait > 0 {
			secs := int(wait / time.Second)
			if wait%time.Second != 0 {
				secs++
			}
			if secs < 1 {
				secs = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(secs))
			if reason == "rate_limit" || reason == "overloaded" {
				http.Error(w, "All providers are rate limited; retry later", http.StatusTooManyRequests)
				return
			}
			http.Error(w, "All providers are temporarily unavailable; retry later", http.StatusServiceUnavailable)
			return
		}
		logger.Error("[%s] all providers unavailable", cp.clientType)
		http.Error(w, "All providers are unavailable", http.StatusServiceUnavailable)
		return
	}

	startIndex := cp.ensureActiveStartIndex()
	attempted := 0

	for offset := 0; offset < len(cp.providers) && attempted < active; offset++ {
		index := (startIndex + offset) % len(cp.providers)
		if cp.isDeactivated(index) {
			continue
		}
		attempted++
		provider := cp.providers[index]

		logger.Debug("[%s] forwarding to: %s (attempt %d/%d)", cp.clientType, provider.Name, attempted, active)

		// Create the proxy request
		proxyReq, err := cp.createProxyRequest(req, provider, path, bodyBytes)
		if err != nil {
			logger.Error("[%s] failed to create request for %s: %v", cp.clientType, provider.Name, err)
			continue
		}

		// Ensure the body can be retried (http.Request may be reused by the transport).
		proxyReq.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(bodyBytes)), nil
		}

		// Send the request
		resp, err := cp.httpClient.Do(proxyReq)
		if err != nil {
			logger.Warn("[%s] %s failed: %v, switching to next provider", cp.clientType, provider.Name, err)
			cp.setCurrentIndex(cp.nextActiveIndex(index))
			continue
		}

		action, reason, msg, cooldown := classifyUpstreamFailure(resp)
		if action != failureReturnToClient {
			resp.Body.Close()
			switch action {
			case failureDeactivateAndRetryNext:
				cp.deactivateFor(index, reason, resp.StatusCode, msg, cp.reactivateAfter)
				logger.Error("[%s] %s deactivated (%s): %d %s", cp.clientType, provider.Name, reason, resp.StatusCode, msg)
			case failureRetryNext:
				if cooldown > 0 {
					cp.deactivateFor(index, reason, resp.StatusCode, msg, cooldown)
					logger.Warn("[%s] %s cooling down for %s (%s): %d %s", cp.clientType, provider.Name, cooldown, reason, resp.StatusCode, msg)
				}
				logger.Warn("[%s] %s failed (%s): %d %s, switching to next provider", cp.clientType, provider.Name, reason, resp.StatusCode, msg)
			}
			cp.setCurrentIndex(cp.nextActiveIndex(index))
			continue
		}

		// Success - copy response to client
		logger.Debug("[%s] request completed via %s", cp.clientType, provider.Name)

		// Update current index to this working provider
		cp.setCurrentIndex(index)

		// Copy headers
		copyHeaders(w.Header(), resp.Header)

		w.WriteHeader(resp.StatusCode)

		// Stream the response body
		if resp.Body != nil {
			if _, err := io.Copy(newFlushWriter(w), resp.Body); err != nil {
				logger.Warn("[%s] response copy failed via %s: %v", cp.clientType, provider.Name, err)
			}
			resp.Body.Close()
		}

		if resp.StatusCode == http.StatusOK {
			logger.Info("[%s] request completed via %s", cp.clientType, provider.Name)
		}
		return
	}

	// If we've cooled down all providers during this request, surface a Retry-After to the client.
	if wait, reason, ok := cp.timeUntilNextAvailable(); ok && wait > 0 && cp.activeProviderCount() == 0 {
		secs := int(wait / time.Second)
		if wait%time.Second != 0 {
			secs++
		}
		if secs < 1 {
			secs = 1
		}
		w.Header().Set("Retry-After", strconv.Itoa(secs))
		if reason == "rate_limit" || reason == "overloaded" {
			http.Error(w, "All providers are rate limited; retry later", http.StatusTooManyRequests)
			return
		}
		http.Error(w, "All providers are temporarily unavailable; retry later", http.StatusServiceUnavailable)
		return
	}

	logger.Error("[%s] all providers failed", cp.clientType)
	http.Error(w, "All providers failed", http.StatusServiceUnavailable)
}

// forwardCountTokensWithFailover forwards Claude Code /v1/messages/count_tokens requests while
// keeping the main conversation provider sticky (cp.currentIndex) unchanged.
//
// Rationale: Claude Code calls count_tokens frequently; using those failures to move the primary
// provider can reduce context-cache effectiveness and increase token usage.
func (cp *ClientProxy) forwardCountTokensWithFailover(w http.ResponseWriter, req *http.Request, path string) {
	cp.reactivateExpired()

	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		logger.Error("[%s] failed to read request body: %v", cp.clientType, err)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

	active := cp.activeProviderCount()
	if active == 0 {
		wait, reason, ok := cp.timeUntilNextAvailable()
		if ok && wait > 0 {
			secs := int(wait / time.Second)
			if wait%time.Second != 0 {
				secs++
			}
			if secs < 1 {
				secs = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(secs))
			if reason == "rate_limit" || reason == "overloaded" {
				http.Error(w, "All providers are rate limited; retry later", http.StatusTooManyRequests)
				return
			}
			http.Error(w, "All providers are temporarily unavailable; retry later", http.StatusServiceUnavailable)
			return
		}
		logger.Error("[%s] all providers unavailable", cp.clientType)
		http.Error(w, "All providers are unavailable", http.StatusServiceUnavailable)
		return
	}

	startIndex := cp.ensureActiveCountTokensStartIndex()
	attempted := 0

	for offset := 0; offset < len(cp.providers) && attempted < active; offset++ {
		index := (startIndex + offset) % len(cp.providers)
		if cp.isDeactivated(index) {
			continue
		}
		attempted++
		provider := cp.providers[index]

		logger.Debug("[%s] forwarding to: %s (count_tokens attempt %d/%d)", cp.clientType, provider.Name, attempted, active)

		proxyReq, err := cp.createProxyRequest(req, provider, path, bodyBytes)
		if err != nil {
			logger.Error("[%s] failed to create request for %s: %v", cp.clientType, provider.Name, err)
			cp.setCountTokensIndex(cp.nextActiveIndex(index))
			continue
		}

		proxyReq.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(bodyBytes)), nil
		}

		resp, err := cp.httpClient.Do(proxyReq)
		if err != nil {
			logger.Warn("[%s] %s failed (count_tokens): %v, trying next provider", cp.clientType, provider.Name, err)
			cp.setCountTokensIndex(cp.nextActiveIndex(index))
			continue
		}

		// For count_tokens, only treat auth/billing failures as hard signals that can deactivate a provider.
		// Other transient failures should not impact the main conversation stickiness (cp.currentIndex).
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			msg := readAndTruncate(resp.Body, 2048)
			resp.Body.Close()
			cp.deactivateFor(index, "auth", resp.StatusCode, msg, cp.reactivateAfter)
			cp.setCountTokensIndex(cp.nextActiveIndex(index))
			logger.Error("[%s] %s deactivated (count_tokens auth): %d %s", cp.clientType, provider.Name, resp.StatusCode, msg)
			continue
		}
		if resp.StatusCode == http.StatusPaymentRequired {
			msg := readAndTruncate(resp.Body, 2048)
			resp.Body.Close()
			cp.deactivateFor(index, "billing", resp.StatusCode, msg, cp.reactivateAfter)
			cp.setCountTokensIndex(cp.nextActiveIndex(index))
			logger.Error("[%s] %s deactivated (count_tokens billing): %d %s", cp.clientType, provider.Name, resp.StatusCode, msg)
			continue
		}
		if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
			msg := readAndTruncate(resp.Body, 2048)
			resp.Body.Close()
			cp.setCountTokensIndex(cp.nextActiveIndex(index))
			logger.Warn("[%s] %s failed (count_tokens): %d %s, trying next provider", cp.clientType, provider.Name, resp.StatusCode, msg)
			continue
		}

		// Success (or any non-retriable response) - return to client and make count_tokens sticky to this provider.
		cp.setCountTokensIndex(index)
		copyHeaders(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)
		if resp.Body != nil {
			if _, err := io.Copy(newFlushWriter(w), resp.Body); err != nil {
				logger.Warn("[%s] response copy failed via %s (count_tokens): %v", cp.clientType, provider.Name, err)
			}
		}
		resp.Body.Close()
		return
	}

	logger.Error("[%s] all providers failed (count_tokens)", cp.clientType)
	http.Error(w, "All providers failed", http.StatusServiceUnavailable)
}

func (cp *ClientProxy) reactivateExpired() {
	now := time.Now()

	cp.mu.Lock()
	defer cp.mu.Unlock()

	if len(cp.deactivated) != len(cp.providers) {
		return
	}
	for i, d := range cp.deactivated {
		if d.until.IsZero() {
			continue
		}
		if now.Before(d.until) {
			continue
		}
		cp.deactivated[i] = providerDeactivation{}
		if i < len(cp.providers) {
			logger.Info("[%s] provider %s reactivated", cp.clientType, cp.providers[i].Name)
		} else {
			logger.Info("[%s] provider #%d reactivated", cp.clientType, i)
		}
	}
}

func (cp *ClientProxy) isDeactivated(index int) bool {
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	if index < 0 || index >= len(cp.deactivated) {
		return false
	}
	d := cp.deactivated[index]
	return !d.until.IsZero() && time.Now().Before(d.until)
}

func (cp *ClientProxy) deactivateFor(index int, reason string, status int, msg string, d time.Duration) {
	if d <= 0 {
		return
	}
	cp.mu.Lock()
	defer cp.mu.Unlock()
	if index < 0 || index >= len(cp.deactivated) {
		return
	}
	if !cp.deactivated[index].until.IsZero() && time.Now().Before(cp.deactivated[index].until) {
		return
	}
	now := time.Now()
	cp.deactivated[index] = providerDeactivation{
		at:      now,
		until:   now.Add(d),
		reason:  reason,
		status:  status,
		message: msg,
	}

	// If the current index was deactivated, move it forward.
	if cp.currentIndex == index {
		cp.currentIndex = cp.nextActiveIndexLocked(index)
	}
}

func (cp *ClientProxy) deactivationUntil(index int) time.Time {
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	if index < 0 || index >= len(cp.deactivated) {
		return time.Time{}
	}
	return cp.deactivated[index].until
}

func (cp *ClientProxy) timeUntilNextAvailable() (wait time.Duration, reason string, ok bool) {
	cp.mu.RLock()
	defer cp.mu.RUnlock()

	now := time.Now()
	var soonest time.Time
	var soonestReason string
	for _, d := range cp.deactivated {
		if d.until.IsZero() || !now.Before(d.until) {
			continue
		}
		if soonest.IsZero() || d.until.Before(soonest) {
			soonest = d.until
			soonestReason = d.reason
		}
	}
	if soonest.IsZero() {
		return 0, "", false
	}
	return time.Until(soonest), soonestReason, true
}

func (cp *ClientProxy) activeProviderCount() int {
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	now := time.Now()
	count := 0
	for i := range cp.providers {
		if cp.deactivated[i].until.IsZero() || !now.Before(cp.deactivated[i].until) {
			count++
		}
	}
	return count
}

func (cp *ClientProxy) ensureActiveStartIndex() int {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	now := time.Now()

	if len(cp.providers) == 0 {
		return 0
	}
	if cp.currentIndex < 0 || cp.currentIndex >= len(cp.providers) {
		cp.currentIndex = 0
	}
	if cp.deactivated[cp.currentIndex].until.IsZero() || !now.Before(cp.deactivated[cp.currentIndex].until) {
		return cp.currentIndex
	}
	cp.currentIndex = cp.nextActiveIndexLocked(cp.currentIndex)
	return cp.currentIndex
}

func (cp *ClientProxy) ensureActiveCountTokensStartIndex() int {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	now := time.Now()

	if len(cp.providers) == 0 {
		return 0
	}
	if cp.countTokensIndex < 0 || cp.countTokensIndex >= len(cp.providers) {
		cp.countTokensIndex = 0
	}
	if cp.deactivated[cp.countTokensIndex].until.IsZero() || !now.Before(cp.deactivated[cp.countTokensIndex].until) {
		return cp.countTokensIndex
	}
	cp.countTokensIndex = cp.nextActiveIndexLocked(cp.countTokensIndex)
	return cp.countTokensIndex
}

func (cp *ClientProxy) setCountTokensIndex(index int) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	cp.countTokensIndex = index
}

func (cp *ClientProxy) nextActiveIndex(from int) int {
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	return cp.nextActiveIndexLocked(from)
}

func (cp *ClientProxy) nextActiveIndexLocked(from int) int {
	n := len(cp.providers)
	if n == 0 {
		return 0
	}
	now := time.Now()
	for step := 1; step <= n; step++ {
		idx := (from + step) % n
		if cp.deactivated[idx].until.IsZero() || !now.Before(cp.deactivated[idx].until) {
			return idx
		}
	}
	// All deactivated; keep current.
	return from % n
}

type failureAction int

const (
	failureReturnToClient failureAction = iota
	failureRetryNext
	failureDeactivateAndRetryNext
)

func classifyUpstreamFailure(resp *http.Response) (action failureAction, reason string, msg string, cooldown time.Duration) {
	status := resp.StatusCode

	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return failureDeactivateAndRetryNext, "auth", readAndTruncate(resp.Body, 2048), 0
	case http.StatusPaymentRequired:
		return failureDeactivateAndRetryNext, "billing", readAndTruncate(resp.Body, 2048), 0
	case http.StatusTooManyRequests:
		body, truncated := readBodyBytes(resp.Body, 32*1024)
		snippet := string(body)
		if truncated {
			snippet += "..."
		}
		cooldown = retryAfterDuration(resp.Header)
		action, reason := classify429(body)
		switch action {
		case failureDeactivateAndRetryNext:
			return action, reason, truncateString(snippet, 2048), 0
		case failureRetryNext:
			return action, reason, truncateString(snippet, 2048), cooldown
		default:
			return failureRetryNext, "rate_limit", truncateString(snippet, 2048), cooldown
		}
	default:
		if shouldRetry(status) {
			cooldown = retryAfterDuration(resp.Header)
			return failureRetryNext, "server", readAndTruncate(resp.Body, 2048), cooldown
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

func readBodyBytes(r io.Reader, maxBytes int64) (data []byte, truncated bool) {
	if maxBytes <= 0 {
		_, _ = io.Copy(io.Discard, r)
		return nil, false
	}
	data, _ = io.ReadAll(io.LimitReader(r, maxBytes+1))
	truncated = int64(len(data)) > maxBytes
	if truncated {
		data = data[:maxBytes]
	}
	_, _ = io.Copy(io.Discard, r)
	return data, truncated
}

func readAndTruncate(r io.Reader, maxBytes int64) string {
	if maxBytes <= 0 {
		return ""
	}
	data, truncated := readBodyBytes(r, maxBytes)
	if truncated {
		return string(data) + "..."
	}
	return string(data)
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
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

type flushWriter struct {
	w http.ResponseWriter
}

func newFlushWriter(w http.ResponseWriter) io.Writer {
	if _, ok := w.(http.Flusher); !ok {
		return w
	}
	return &flushWriter{w: w}
}

func (fw *flushWriter) Write(p []byte) (int, error) {
	n, err := fw.w.Write(p)
	if fl, ok := fw.w.(http.Flusher); ok {
		fl.Flush()
	}
	return n, err
}
