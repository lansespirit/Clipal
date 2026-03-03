package proxy

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/lansespirit/Clipal/internal/config"
	"github.com/lansespirit/Clipal/internal/logger"
	"github.com/lansespirit/Clipal/internal/notify"
)

func nextProviderName(cp *ClientProxy, fromIndex int) (idx int, name string) {
	idx = cp.nextActiveIndex(fromIndex)
	if idx == fromIndex {
		return idx, ""
	}
	if idx < 0 || idx >= len(cp.providers) {
		return idx, ""
	}
	n := strings.TrimSpace(cp.providers[idx].Name)
	if n == "" {
		return idx, ""
	}
	return idx, n
}

// forwardWithFailover forwards the request with automatic failover.
func (cp *ClientProxy) forwardWithFailover(w http.ResponseWriter, req *http.Request, path string) {
	if cp.mode == config.ClientModeManual {
		cp.forwardManual(w, req, path)
		return
	}

	cp.reactivateExpired()

	// If the client has already gone away (or the server is shutting down), don't do any work.
	if err := req.Context().Err(); err != nil {
		return
	}

	// Read the request body once for potential retries
	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		logger.Error("[%s] failed to read request body: %v", cp.clientType, err)
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeProxyError(w, "Request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		writeProxyError(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

	// Atomically get active count and start index to avoid TOCTOU race.
	active, startIndex := cp.getActiveCountAndStartIndex()
	if active == 0 {
		if handled := cp.handleAllUnavailable(w); handled {
			return
		}
		logger.Error("[%s] all providers unavailable", cp.clientType)
		writeProxyError(w, "All providers are unavailable", http.StatusServiceUnavailable)
		return
	}

	attempted := 0
	originProvider := ""
	lastSwitchReason := ""
	lastSwitchStatus := 0

	for offset := 0; offset < len(cp.providers) && attempted < active; offset++ {
		if err := req.Context().Err(); err != nil {
			return
		}

		index := (startIndex + offset) % len(cp.providers)
		if cp.isDeactivated(index) {
			continue
		}
		now := time.Now()
		allow := cp.allowCircuit(now, index)
		if !allow.allowed {
			continue
		}
		provider := cp.providers[index]
		if originProvider == "" {
			originProvider = provider.Name
		}

		attempted++
		logger.Debug("[%s] forwarding to: %s (attempt %d/%d)", cp.clientType, provider.Name, attempted, active)

		attemptCtx, cancelAttempt := context.WithCancelCause(req.Context())

		// Create the proxy request
		reqWithAttemptCtx := req.WithContext(attemptCtx)
		proxyReq, err := cp.createProxyRequest(reqWithAttemptCtx, provider, path, bodyBytes)
		if err != nil {
			logger.Error("[%s] failed to create request for %s: %v", cp.clientType, provider.Name, err)
			cp.releaseCircuitPermit(index, allow.usedProbe)
			cancelAttempt(nil)
			continue
		}

		// Ensure the body can be retried (http.Request may be reused by the transport).
		proxyReq.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(bodyBytes)), nil
		}

		// Send the request
		resp, err := cp.httpClient.Do(proxyReq)
		if err != nil {
			// Don't retry across providers when the request context is already canceled;
			// this otherwise produces misleading "all providers failed" logs.
			if req.Context().Err() != nil {
				cp.releaseCircuitPermit(index, allow.usedProbe)
				cancelAttempt(nil)
				return
			}
			cp.recordCircuitFailure(time.Now(), index, allow.usedProbe, "network")
			cancelAttempt(nil)
			nextIndex, nextName := nextProviderName(cp, index)
			if nextName != "" {
				logger.Warn("[%s] %s failed (network): %v; trying next=%s", cp.clientType, provider.Name, err, nextName)
			} else {
				logger.Warn("[%s] %s failed (network): %v; trying next provider", cp.clientType, provider.Name, err)
			}
			lastSwitchReason = "network"
			lastSwitchStatus = 0
			cp.setCurrentIndex(nextIndex)
			continue
		}

		var (
			action   failureAction
			reason   string
			msg      string
			cooldown time.Duration
		)
		inspect := resp.StatusCode == http.StatusUnauthorized ||
			resp.StatusCode == http.StatusForbidden ||
			resp.StatusCode == http.StatusPaymentRequired ||
			resp.StatusCode == http.StatusTooManyRequests ||
			shouldRetry(resp.StatusCode)
		if inspect {
			body, truncated := readResponseBodyBytes(resp, 32*1024)
			action, reason, msg, cooldown = classifyUpstreamFailure(resp.StatusCode, resp.Header, body, truncated)
		} else {
			action = failureReturnToClient
		}

		if action != failureReturnToClient {
			cp.recordCircuitFailureFromClassification(time.Now(), index, allow.usedProbe, reason)
			resp.Body.Close()
			cancelAttempt(nil)
			lastSwitchReason = reason
			lastSwitchStatus = resp.StatusCode
			nextIndex, nextName := nextProviderName(cp, index)
			switch action {
			case failureDeactivateAndRetryNext:
				cp.deactivateFor(index, reason, resp.StatusCode, msg, cp.reactivateAfter)
				if nextName != "" {
					logger.Error("[%s] %s deactivated (%s): %d %s; trying next=%s", cp.clientType, provider.Name, reason, resp.StatusCode, msg, nextName)
				} else {
					logger.Error("[%s] %s deactivated (%s): %d %s; trying next provider", cp.clientType, provider.Name, reason, resp.StatusCode, msg)
				}
			case failureRetryNext:
				if cooldown > 0 {
					cp.deactivateFor(index, reason, resp.StatusCode, msg, cooldown)
					logger.Warn("[%s] %s cooling down for %s (%s): %d %s", cp.clientType, provider.Name, cooldown, reason, resp.StatusCode, msg)
				}
				if nextName != "" {
					logger.Warn("[%s] %s failed (%s): %d %s; trying next=%s", cp.clientType, provider.Name, reason, resp.StatusCode, msg, nextName)
				} else {
					logger.Warn("[%s] %s failed (%s): %d %s; trying next provider", cp.clientType, provider.Name, reason, resp.StatusCode, msg)
				}
			}
			cp.setCurrentIndex(nextIndex)
			continue
		}

		// Success (or pass-through response) - copy response to client. For streaming endpoints
		// (SSE), wait for the first body bytes before sending headers so we can fail over cleanly
		// if the upstream hangs after headers.
		// Found a working provider.
		onCommit := func() {
			cp.setCurrentIndex(index)
			if attempted > 1 && originProvider != "" && originProvider != provider.Name {
				logger.Info("[%s] provider switched: %s -> %s (%s %d)", cp.clientType, originProvider, provider.Name, lastSwitchReason, lastSwitchStatus)
				cp.recordProviderSwitch(originProvider, provider.Name, lastSwitchReason, lastSwitchStatus)
				notify.ProviderSwitched(string(cp.clientType), originProvider, provider.Name, lastSwitchReason, lastSwitchStatus)
			}
		}

		if committed := cp.streamResponseToClient(w, resp, req, attemptCtx, cancelAttempt, index, allow, onCommit); committed {
			if resp.StatusCode == http.StatusOK {
				logger.Info("[%s] request completed via %s", cp.clientType, provider.Name)
			}
			return
		}

		// Failed before committing headers (e.g. idle timeout during first byte read).
		// Failover to next provider.
		if isUpstreamIdleTimeout(attemptCtx, nil) {
			logger.Warn("[%s] %s no body bytes for %s, switching to next provider", cp.clientType, provider.Name, cp.upstreamIdle)
			lastSwitchReason = "idle_timeout"
			lastSwitchStatus = 0
			cp.recordCircuitFailure(time.Now(), index, allow.usedProbe, "idle_timeout")
		} else {
			logger.Warn("[%s] %s response read failed before body; switching to next provider", cp.clientType, provider.Name)
			lastSwitchReason = "network"
			lastSwitchStatus = 0
			cp.recordCircuitFailure(time.Now(), index, allow.usedProbe, "network")
		}
		cancelAttempt(nil)
		cp.setCurrentIndex(cp.nextActiveIndex(index))
		continue
	}

	// If we've cooled down all providers during this request, surface a Retry-After to the client.
	if cp.activeProviderCount() == 0 {
		if handled := cp.handleAllUnavailable(w); handled {
			return
		}
	}

	logger.Error("[%s] all providers failed", cp.clientType)
	writeProxyError(w, "All providers failed", http.StatusServiceUnavailable)
}

// forwardCountTokensWithFailover forwards Claude Code /v1/messages/count_tokens requests while
// keeping the main conversation provider sticky (cp.currentIndex) unchanged.
//
// Rationale: Claude Code calls count_tokens frequently; using those failures to move the primary
// provider can reduce context-cache effectiveness and increase token usage.
func (cp *ClientProxy) forwardCountTokensWithFailover(w http.ResponseWriter, req *http.Request, path string) {
	if cp.mode == config.ClientModeManual {
		cp.forwardManual(w, req, path)
		return
	}

	cp.reactivateExpired()

	if err := req.Context().Err(); err != nil {
		return
	}

	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		logger.Error("[%s] failed to read request body: %v", cp.clientType, err)
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeProxyError(w, "Request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		writeProxyError(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

	// Atomically get active count and start index to avoid TOCTOU race.
	active, startIndex := cp.getActiveCountAndCountTokensStartIndex()
	if active == 0 {
		if handled := cp.handleAllUnavailable(w); handled {
			return
		}
		logger.Error("[%s] all providers unavailable", cp.clientType)
		writeProxyError(w, "All providers are unavailable", http.StatusServiceUnavailable)
		return
	}

	attempted := 0

	for offset := 0; offset < len(cp.providers) && attempted < active; offset++ {
		if err := req.Context().Err(); err != nil {
			return
		}

		index := (startIndex + offset) % len(cp.providers)
		if cp.isDeactivated(index) {
			continue
		}
		now := time.Now()
		allow := cp.allowCircuit(now, index)
		if !allow.allowed {
			continue
		}
		attempted++
		provider := cp.providers[index]

		logger.Debug("[%s] forwarding to: %s (count_tokens attempt %d/%d)", cp.clientType, provider.Name, attempted, active)

		attemptCtx, cancelAttempt := context.WithCancelCause(req.Context())
		reqWithAttemptCtx := req.WithContext(attemptCtx)

		proxyReq, err := cp.createProxyRequest(reqWithAttemptCtx, provider, path, bodyBytes)
		if err != nil {
			logger.Error("[%s] failed to create request for %s: %v", cp.clientType, provider.Name, err)
			cp.setCountTokensIndex(cp.nextActiveIndex(index))
			cp.releaseCircuitPermit(index, allow.usedProbe)
			cancelAttempt(nil)
			continue
		}

		proxyReq.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(bodyBytes)), nil
		}

		resp, err := cp.httpClient.Do(proxyReq)
		if err != nil {
			if req.Context().Err() != nil {
				cp.releaseCircuitPermit(index, allow.usedProbe)
				cancelAttempt(nil)
				return
			}
			cp.recordCircuitFailure(time.Now(), index, allow.usedProbe, "network")
			cancelAttempt(nil)
			nextIndex, nextName := nextProviderName(cp, index)
			if nextName != "" {
				logger.Warn("[%s] %s failed (count_tokens network): %v; trying next=%s", cp.clientType, provider.Name, err, nextName)
			} else {
				logger.Warn("[%s] %s failed (count_tokens network): %v; trying next provider", cp.clientType, provider.Name, err)
			}
			cp.setCountTokensIndex(nextIndex)
			continue
		}

		// For count_tokens, only treat auth/billing failures as hard signals that can deactivate a provider.
		// Other transient failures should not impact the main conversation stickiness (cp.currentIndex).
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			msg := readAndTruncateResponse(resp, 2048)
			resp.Body.Close()
			cp.deactivateFor(index, "auth", resp.StatusCode, msg, cp.reactivateAfter)
			cp.recordCircuitFailureFromClassification(time.Now(), index, allow.usedProbe, "auth")
			nextIndex, nextName := nextProviderName(cp, index)
			cp.setCountTokensIndex(nextIndex)
			if nextName != "" {
				logger.Error("[%s] %s deactivated (count_tokens auth): %d %s; trying next=%s", cp.clientType, provider.Name, resp.StatusCode, msg, nextName)
			} else {
				logger.Error("[%s] %s deactivated (count_tokens auth): %d %s; trying next provider", cp.clientType, provider.Name, resp.StatusCode, msg)
			}
			cancelAttempt(nil)
			continue
		}
		if resp.StatusCode == http.StatusPaymentRequired {
			msg := readAndTruncateResponse(resp, 2048)
			resp.Body.Close()
			cp.deactivateFor(index, "billing", resp.StatusCode, msg, cp.reactivateAfter)
			cp.recordCircuitFailureFromClassification(time.Now(), index, allow.usedProbe, "billing")
			nextIndex, nextName := nextProviderName(cp, index)
			cp.setCountTokensIndex(nextIndex)
			if nextName != "" {
				logger.Error("[%s] %s deactivated (count_tokens billing): %d %s; trying next=%s", cp.clientType, provider.Name, resp.StatusCode, msg, nextName)
			} else {
				logger.Error("[%s] %s deactivated (count_tokens billing): %d %s; trying next provider", cp.clientType, provider.Name, resp.StatusCode, msg)
			}
			cancelAttempt(nil)
			continue
		}
		if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
			msg := readAndTruncateResponse(resp, 2048)
			resp.Body.Close()
			if resp.StatusCode >= 500 {
				cp.recordCircuitFailureFromClassification(time.Now(), index, allow.usedProbe, "server")
			} else {
				cp.recordCircuitFailureFromClassification(time.Now(), index, allow.usedProbe, "rate_limit")
			}
			nextIndex, nextName := nextProviderName(cp, index)
			cp.setCountTokensIndex(nextIndex)
			if nextName != "" {
				logger.Warn("[%s] %s failed (count_tokens): %d %s; trying next=%s", cp.clientType, provider.Name, resp.StatusCode, msg, nextName)
			} else {
				logger.Warn("[%s] %s failed (count_tokens): %d %s; trying next provider", cp.clientType, provider.Name, resp.StatusCode, msg)
			}
			cancelAttempt(nil)
			continue
		}

		// Success (or any non-retriable response) - return to client and make count_tokens sticky.
		onCommit := func() {
			cp.setCountTokensIndex(index)
		}

		if committed := cp.streamResponseToClient(w, resp, req, attemptCtx, cancelAttempt, index, allow, onCommit); committed {
			return
		}

		// Failed before committing headers (count_tokens: try next provider).
		cancelAttempt(nil)
		if req.Context().Err() != nil {
			cp.releaseCircuitPermit(index, allow.usedProbe)
			return
		}
		logger.Warn("[%s] %s response read failed before body (count_tokens), trying next provider", cp.clientType, provider.Name)
		if isUpstreamIdleTimeout(attemptCtx, nil) {
			cp.recordCircuitFailure(time.Now(), index, allow.usedProbe, "idle_timeout")
		} else {
			cp.recordCircuitFailure(time.Now(), index, allow.usedProbe, "network")
		}
		cp.setCountTokensIndex(cp.nextActiveIndex(index))
		continue
	}

	logger.Error("[%s] all providers failed (count_tokens)", cp.clientType)
	writeProxyError(w, "All providers failed", http.StatusServiceUnavailable)
}
