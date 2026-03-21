package proxy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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

func (cp *ClientProxy) announceProviderSwitch(from string, to string, reason string, status int) {
	from = strings.TrimSpace(from)
	to = strings.TrimSpace(to)
	if from == "" || to == "" || from == to {
		return
	}
	switchView := DescribeProviderSwitch(from, to, reason, status)
	logger.Info("[%s] %s. %s", cp.clientType, switchView.Label, switchView.Detail)
	cp.recordProviderSwitch(from, to, reason, status)
	notify.ProviderSwitched(string(cp.clientType), switchView.Label, switchView.Detail)
}

func unavailableRequestStatus(reason string) (result string, status int, detail string) {
	if reason == "rate_limit" || reason == "overloaded" {
		return "all_providers_unavailable", http.StatusTooManyRequests, "All providers are rate limited; retry later."
	}
	return "all_providers_unavailable", http.StatusServiceUnavailable, "All providers are temporarily unavailable; retry later."
}

func describeRequestBuildFailure(provider string, err error) string {
	msg := "local request setup failed"
	if err != nil {
		msg = truncateString(sanitizeLogString(err.Error()), 512)
	}
	if strings.TrimSpace(provider) == "" {
		return "Request could not be prepared locally: " + msg
	}
	return fmt.Sprintf("%s request could not be prepared locally: %s", provider, msg)
}

func isKeyScopedFailure(reason string) bool {
	switch reason {
	case "auth", "billing", "quota", "rate_limit", "overloaded":
		return true
	default:
		return false
	}
}

func keyFailureDuration(reason string, cooldown time.Duration, reactivateAfter time.Duration) time.Duration {
	switch reason {
	case "auth", "billing", "quota":
		return reactivateAfter
	case "rate_limit", "overloaded":
		return cooldown
	default:
		return 0
	}
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
			cp.recordTerminalRequest(time.Now(), "", http.StatusRequestEntityTooLarge, "request_rejected", "Request body too large.")
			writeProxyError(w, "Request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		cp.recordTerminalRequest(time.Now(), "", http.StatusBadRequest, "request_rejected", "Failed to read request body.")
		writeProxyError(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer func() { _ = req.Body.Close() }()

	// Atomically get active count and start index to avoid TOCTOU race.
	active, startIndex := cp.getActiveCountAndStartIndex()
	if active == 0 {
		if wait, reason, ok := cp.timeUntilNextAvailable(); ok && wait > 0 {
			result, status, detail := unavailableRequestStatus(reason)
			cp.recordTerminalRequest(time.Now(), "", status, result, detail)
		} else {
			cp.recordTerminalRequest(time.Now(), "", http.StatusServiceUnavailable, "all_providers_unavailable", "All providers are unavailable.")
		}
		if handled := cp.handleAllUnavailable(w); handled {
			return
		}
		logger.Error("[%s] all providers unavailable", cp.clientType)
		writeProxyError(w, "All providers are unavailable", http.StatusServiceUnavailable)
		return
	}

	attempted := 0
	lastSwitchReason := ""
	lastSwitchStatus := 0
	lastFailedProvider := ""
	attemptSummaries := make([]string, 0, active)
	hadUpstreamAttempt := false

	for offset := 0; offset < len(cp.providers) && attempted < active; offset++ {
		if err := req.Context().Err(); err != nil {
			return
		}

		index := (startIndex + offset) % len(cp.providers)
		if cp.isDeactivated(index) || cp.activeKeyCount(index) == 0 {
			continue
		}
		now := time.Now()
		allow := cp.allowCircuit(now, index)
		if !allow.allowed {
			continue
		}
		provider := cp.providers[index]
		keyActive, keyStart := cp.getActiveKeyCountAndStartIndex(index, false)
		if keyActive == 0 {
			cp.releaseCircuitPermit(index, allow.usedProbe)
			continue
		}

		attempted++
		logger.Debug("[%s] forwarding to: %s (attempt %d/%d, keys=%d)", cp.clientType, provider.Name, attempted, active, len(cp.providerKeys[index]))

		providerFailed := false
		keyExhausted := false
		keyExhaustedReason := ""
		keyExhaustedStatus := 0

		for keyOffset, keyTried := 0, 0; keyOffset < len(cp.providerKeys[index]) && keyTried < keyActive; keyOffset++ {
			keyIndex := (keyStart + keyOffset) % len(cp.providerKeys[index])
			if cp.isKeyDeactivated(index, keyIndex) {
				continue
			}
			keyTried++
			apiKey := cp.providerKeys[index][keyIndex]

			attemptCtx, cancelAttempt := context.WithCancelCause(req.Context())
			reqWithAttemptCtx := req.WithContext(attemptCtx)
			proxyReq, err := cp.createProxyRequest(reqWithAttemptCtx, provider, apiKey, path, bodyBytes)
			if err != nil {
				summary := describeRequestBuildFailure(provider.Name, err)
				attemptSummaries = append(attemptSummaries, summary)
				lastFailedProvider = provider.Name
				logger.Error("[%s] %s", cp.clientType, summary)
				cp.releaseCircuitPermit(index, allow.usedProbe)
				cancelAttempt(nil)
				providerFailed = true
				break
			}
			hadUpstreamAttempt = true

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
				summary := describeAttemptFailure(provider.Name, "network", 0, true)
				attemptSummaries = append(attemptSummaries, summary)
				if nextName != "" {
					logger.Warn("[%s] %s; trying next=%s", cp.clientType, summary, nextName)
				} else {
					logger.Warn("[%s] %s; trying next provider", cp.clientType, summary)
				}
				lastSwitchReason = "network"
				lastSwitchStatus = 0
				lastFailedProvider = provider.Name
				if nextName != "" {
					cp.announceProviderSwitch(provider.Name, nextName, lastSwitchReason, lastSwitchStatus)
				}
				cp.setCurrentIndex(nextIndex)
				providerFailed = true
				break
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
				_ = resp.Body.Close()
				cancelAttempt(nil)
				lastFailedProvider = provider.Name
				summary := describeAttemptFailure(provider.Name, reason, resp.StatusCode, false)
				attemptSummaries = append(attemptSummaries, summary)
				if isKeyScopedFailure(reason) {
					d := keyFailureDuration(reason, cooldown, cp.reactivateAfter)
					if d > 0 {
						cp.deactivateKeyFor(index, keyIndex, reason, resp.StatusCode, msg, d)
					}
					nextKeyActive := cp.activeKeyCount(index)
					if nextKeyActive > 0 {
						nextKeyIndex := cp.nextActiveKeyIndex(index, keyIndex)
						cp.setCurrentKeyIndex(index, nextKeyIndex)
						if action == failureDeactivateAndRetryNext {
							logger.Error("[%s] %s; trying next key for provider=%s", cp.clientType, summary, provider.Name)
						} else {
							logger.Warn("[%s] %s; trying next key for provider=%s", cp.clientType, summary, provider.Name)
						}
						continue
					}
					if d > 0 {
						cp.deactivateFor(index, reason, resp.StatusCode, msg, d)
					}
					cp.recordCircuitFailureFromClassification(time.Now(), index, allow.usedProbe, reason)
					keyExhausted = true
					keyExhaustedReason = reason
					keyExhaustedStatus = resp.StatusCode
					break
				}

				cp.recordCircuitFailureFromClassification(time.Now(), index, allow.usedProbe, reason)
				lastSwitchReason = reason
				lastSwitchStatus = resp.StatusCode
				nextIndex, nextName := nextProviderName(cp, index)
				switch action {
				case failureDeactivateAndRetryNext:
					cp.deactivateFor(index, reason, resp.StatusCode, msg, cp.reactivateAfter)
					if nextName != "" {
						logger.Error("[%s] %s; marking provider unavailable and trying next=%s", cp.clientType, summary, nextName)
					} else {
						logger.Error("[%s] %s; marking provider unavailable and trying next provider", cp.clientType, summary)
					}
				case failureRetryNext:
					summaryWithCooldown := summary
					if cooldown > 0 {
						cp.deactivateFor(index, reason, resp.StatusCode, msg, cooldown)
						summaryWithCooldown = fmt.Sprintf("%s; cooling down for %s", summary, cooldown)
					}
					if nextName != "" {
						logger.Warn("[%s] %s; trying next=%s", cp.clientType, summaryWithCooldown, nextName)
					} else {
						logger.Warn("[%s] %s; trying next provider", cp.clientType, summaryWithCooldown)
					}
				}
				if nextName != "" {
					cp.announceProviderSwitch(provider.Name, nextName, lastSwitchReason, lastSwitchStatus)
				}
				cp.setCurrentIndex(nextIndex)
				providerFailed = true
				break
			}

			onCommit := func() {
				cp.setCurrentIndex(index)
				cp.setCurrentKeyIndex(index, keyIndex)
			}

			result := cp.streamResponseToClient(w, resp, req, attemptCtx, cancelAttempt, index, allow, onCommit)
			if result.kind == streamFinal {
				cp.logRequestResult(provider.Name, resp.StatusCode, result, false)
				return
			}

			if isUpstreamIdleTimeout(attemptCtx, attemptCtx.Err()) {
				summary := describeAttemptFailure(provider.Name, "idle_timeout", 0, true)
				attemptSummaries = append(attemptSummaries, summary)
				logger.Warn("[%s] %s; trying next provider", cp.clientType, summary)
				lastSwitchReason = "idle_timeout"
				lastSwitchStatus = 0
				lastFailedProvider = provider.Name
				cp.recordCircuitFailure(time.Now(), index, allow.usedProbe, "idle_timeout")
			} else {
				summary := describeAttemptFailure(provider.Name, "network", 0, true)
				attemptSummaries = append(attemptSummaries, summary)
				logger.Warn("[%s] %s; trying next provider", cp.clientType, summary)
				lastSwitchReason = "network"
				lastSwitchStatus = 0
				lastFailedProvider = provider.Name
				cp.recordCircuitFailure(time.Now(), index, allow.usedProbe, "network")
			}
			cancelAttempt(nil)
			nextIndex, nextName := nextProviderName(cp, index)
			if nextName != "" {
				cp.announceProviderSwitch(provider.Name, nextName, lastSwitchReason, lastSwitchStatus)
			}
			cp.setCurrentIndex(nextIndex)
			providerFailed = true
			break
		}

		if providerFailed {
			continue
		}
		if keyExhausted {
			lastFailedProvider = provider.Name
			nextIndex, nextName := nextProviderName(cp, index)
			if nextName != "" {
				logger.Warn("[%s] provider %s exhausted available keys; trying next=%s", cp.clientType, provider.Name, nextName)
				cp.announceProviderSwitch(provider.Name, nextName, keyExhaustedReason, keyExhaustedStatus)
			} else {
				logger.Warn("[%s] provider %s exhausted available keys; trying next provider", cp.clientType, provider.Name)
			}
			cp.setCurrentIndex(nextIndex)
		}
	}

	// If we've cooled down all providers during this request, surface a Retry-After to the client.
	if cp.activeProviderCount() == 0 {
		if wait, reason, ok := cp.timeUntilNextAvailable(); ok && wait > 0 {
			result, status, detail := unavailableRequestStatus(reason)
			cp.recordTerminalRequest(time.Now(), "", status, result, detail)
		}
		if handled := cp.handleAllUnavailable(w); handled {
			return
		}
	}

	lastProvider := strings.TrimSpace(lastFailedProvider)
	terminalResult := "all_providers_failed"
	terminalStatus := http.StatusServiceUnavailable
	if !hadUpstreamAttempt {
		terminalResult = "request_rejected"
		terminalStatus = http.StatusBadGateway
	}
	cp.recordTerminalRequest(time.Now(), lastProvider, terminalStatus, terminalResult, strings.Join(attemptSummaries, "; "))
	if len(attemptSummaries) > 0 {
		logger.Error("[%s] all providers failed: %s", cp.clientType, strings.Join(attemptSummaries, "; "))
	} else {
		logger.Error("[%s] all providers failed", cp.clientType)
	}
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
	defer func() { _ = req.Body.Close() }()

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
	attemptSummaries := make([]string, 0, active)
	hadUpstreamAttempt := false

	for offset := 0; offset < len(cp.providers) && attempted < active; offset++ {
		if err := req.Context().Err(); err != nil {
			return
		}

		index := (startIndex + offset) % len(cp.providers)
		if cp.isDeactivated(index) || cp.activeKeyCount(index) == 0 {
			continue
		}
		now := time.Now()
		allow := cp.allowCircuit(now, index)
		if !allow.allowed {
			continue
		}
		attempted++
		provider := cp.providers[index]
		keyActive, keyStart := cp.getActiveKeyCountAndStartIndex(index, true)
		if keyActive == 0 {
			cp.releaseCircuitPermit(index, allow.usedProbe)
			continue
		}

		logger.Debug("[%s] forwarding to: %s (count_tokens attempt %d/%d, keys=%d)", cp.clientType, provider.Name, attempted, active, len(cp.providerKeys[index]))

		providerFailed := false
		keyExhausted := false

		for keyOffset, keyTried := 0, 0; keyOffset < len(cp.providerKeys[index]) && keyTried < keyActive; keyOffset++ {
			keyIndex := (keyStart + keyOffset) % len(cp.providerKeys[index])
			if cp.isKeyDeactivated(index, keyIndex) {
				continue
			}
			keyTried++
			apiKey := cp.providerKeys[index][keyIndex]

			attemptCtx, cancelAttempt := context.WithCancelCause(req.Context())
			reqWithAttemptCtx := req.WithContext(attemptCtx)

			proxyReq, err := cp.createProxyRequest(reqWithAttemptCtx, provider, apiKey, path, bodyBytes)
			if err != nil {
				summary := describeRequestBuildFailure(provider.Name, err)
				attemptSummaries = append(attemptSummaries, summary)
				logger.Error("[%s] %s during count_tokens", cp.clientType, summary)
				cp.setCountTokensIndex(cp.nextActiveIndex(index))
				cp.releaseCircuitPermit(index, allow.usedProbe)
				cancelAttempt(nil)
				providerFailed = true
				break
			}
			hadUpstreamAttempt = true

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
				summary := describeAttemptFailure(provider.Name, "network", 0, true)
				attemptSummaries = append(attemptSummaries, summary)
				if nextName != "" {
					logger.Warn("[%s] %s during count_tokens; trying next=%s", cp.clientType, summary, nextName)
				} else {
					logger.Warn("[%s] %s during count_tokens; trying next provider", cp.clientType, summary)
				}
				cp.setCountTokensIndex(nextIndex)
				providerFailed = true
				break
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
				_ = resp.Body.Close()
				cancelAttempt(nil)
				summary := describeAttemptFailure(provider.Name, reason, resp.StatusCode, false)
				attemptSummaries = append(attemptSummaries, summary)
				if isKeyScopedFailure(reason) {
					d := keyFailureDuration(reason, cooldown, cp.reactivateAfter)
					if d > 0 {
						cp.deactivateKeyFor(index, keyIndex, reason, resp.StatusCode, msg, d)
					}
					if cp.activeKeyCount(index) > 0 {
						nextKeyIndex := cp.nextActiveKeyIndex(index, keyIndex)
						cp.setCountTokensKeyIndex(index, nextKeyIndex)
						if action == failureDeactivateAndRetryNext {
							logger.Error("[%s] %s during count_tokens; trying next key for provider=%s", cp.clientType, summary, provider.Name)
						} else {
							logger.Warn("[%s] %s during count_tokens; trying next key for provider=%s", cp.clientType, summary, provider.Name)
						}
						continue
					}
					if d > 0 {
						cp.deactivateFor(index, reason, resp.StatusCode, msg, d)
					}
					cp.recordCircuitFailureFromClassification(time.Now(), index, allow.usedProbe, reason)
					keyExhausted = true
					break
				}

				cp.recordCircuitFailureFromClassification(time.Now(), index, allow.usedProbe, reason)
				nextIndex, nextName := nextProviderName(cp, index)
				cp.setCountTokensIndex(nextIndex)
				switch action {
				case failureDeactivateAndRetryNext:
					cp.deactivateFor(index, reason, resp.StatusCode, msg, cp.reactivateAfter)
					if nextName != "" {
						logger.Error("[%s] %s during count_tokens; marking provider unavailable and trying next=%s", cp.clientType, summary, nextName)
					} else {
						logger.Error("[%s] %s during count_tokens; marking provider unavailable and trying next provider", cp.clientType, summary)
					}
				case failureRetryNext:
					if nextName != "" {
						logger.Warn("[%s] %s during count_tokens; trying next=%s", cp.clientType, summary, nextName)
					} else {
						logger.Warn("[%s] %s during count_tokens; trying next provider", cp.clientType, summary)
					}
				}
				providerFailed = true
				break
			}

			onCommit := func() {
				cp.setCountTokensIndex(index)
				cp.setCountTokensKeyIndex(index, keyIndex)
			}

			result := cp.streamResponseToClient(w, resp, req, attemptCtx, cancelAttempt, index, allow, onCommit)
			if result.kind == streamFinal {
				return
			}

			cancelAttempt(nil)
			if req.Context().Err() != nil {
				cp.releaseCircuitPermit(index, allow.usedProbe)
				return
			}
			reason = "network"
			if isUpstreamIdleTimeout(attemptCtx, attemptCtx.Err()) {
				reason = "idle_timeout"
				cp.recordCircuitFailure(time.Now(), index, allow.usedProbe, "idle_timeout")
			} else {
				cp.recordCircuitFailure(time.Now(), index, allow.usedProbe, "network")
			}
			summary := describeAttemptFailure(provider.Name, reason, 0, true)
			attemptSummaries = append(attemptSummaries, summary)
			logger.Warn("[%s] %s during count_tokens; trying next provider", cp.clientType, summary)
			cp.setCountTokensIndex(cp.nextActiveIndex(index))
			providerFailed = true
			break
		}

		if providerFailed {
			continue
		}
		if keyExhausted {
			nextIndex, nextName := nextProviderName(cp, index)
			cp.setCountTokensIndex(nextIndex)
			if nextName != "" {
				logger.Warn("[%s] provider %s exhausted available keys during count_tokens; trying next=%s", cp.clientType, provider.Name, nextName)
			} else {
				logger.Warn("[%s] provider %s exhausted available keys during count_tokens; trying next provider", cp.clientType, provider.Name)
			}
		}
	}

	if len(attemptSummaries) > 0 {
		logger.Error("[%s] all providers failed during count_tokens: %s", cp.clientType, strings.Join(attemptSummaries, "; "))
	} else if !hadUpstreamAttempt {
		logger.Error("[%s] count_tokens request could not be prepared for any provider", cp.clientType)
	} else {
		logger.Error("[%s] all providers failed (count_tokens)", cp.clientType)
	}
	writeProxyError(w, "All providers failed", http.StatusServiceUnavailable)
}
