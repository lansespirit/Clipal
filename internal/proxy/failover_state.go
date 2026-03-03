package proxy

import (
	"net/http"
	"strconv"
	"time"

	"github.com/lansespirit/Clipal/internal/config"
	"github.com/lansespirit/Clipal/internal/logger"
)

type providerDeactivation struct {
	at      time.Time
	until   time.Time
	reason  string
	status  int
	message string
}

func setRetryAfterHeader(w http.ResponseWriter, wait time.Duration) {
	// Best-effort: include Retry-After for retryable errors.
	// If a handler already set it, keep that value.
	if w.Header().Get("Retry-After") != "" {
		return
	}
	secs := 1
	if wait > 0 {
		secs = int(wait/time.Second) + 1
		if secs < 1 {
			secs = 1
		}
	}
	w.Header().Set("Retry-After", strconv.Itoa(secs))
}

func writeProxyError(w http.ResponseWriter, msg string, status int) {
	// Only attach Retry-After for retryable errors we generate locally.
	// Upstream responses are streamed through as-is.
	if status == http.StatusTooManyRequests || shouldRetry(status) {
		setRetryAfterHeader(w, time.Second)
	}
	http.Error(w, msg, status)
}

func (cp *ClientProxy) reactivateExpired() {
	now := time.Now()

	cp.mu.Lock()
	defer cp.mu.Unlock()

	if len(cp.deactivated) != len(cp.providers) {
		// This indicates a bug in initialization; log it for debugging.
		logger.Error("[%s] deactivated slice length mismatch: %d != %d (providers)",
			cp.clientType, len(cp.deactivated), len(cp.providers))
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
	if cp.mode != config.ClientModeManual && cp.currentIndex == index {
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

func (cp *ClientProxy) timeUntilProviderAvailable(index int) (wait time.Duration, reason string, ok bool) {
	now := time.Now()

	cp.mu.RLock()
	var d providerDeactivation
	if index >= 0 && index < len(cp.deactivated) {
		d = cp.deactivated[index]
	}
	var cb *circuitBreaker
	if index >= 0 && index < len(cp.breakers) {
		cb = cp.breakers[index]
	}
	cp.mu.RUnlock()

	if !d.until.IsZero() && now.Before(d.until) {
		return time.Until(d.until), d.reason, true
	}
	if cb != nil {
		allow := cb.peekAllow(now)
		if !allow.allowed && allow.wait > 0 {
			return allow.wait, string(allow.reason), true
		}
	}
	return 0, "", false
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
	for _, cb := range cp.breakers {
		if cb == nil {
			continue
		}
		allow := cb.peekAllow(now)
		if allow.allowed || allow.wait <= 0 {
			continue
		}
		until := now.Add(allow.wait)
		if soonest.IsZero() || until.Before(soonest) {
			soonest = until
			soonestReason = string(allow.reason)
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
		// Positive logic: provider is active when not deactivated.
		// Matches the pattern used in getActiveCountAndStartIndex.
		if !cp.deactivated[i].until.IsZero() && now.Before(cp.deactivated[i].until) {
			continue
		}
		if i < len(cp.breakers) && cp.breakers[i] != nil {
			allow := cp.breakers[i].peekAllow(now)
			if !allow.allowed && allow.wait > 0 {
				continue
			}
		}
		count++
	}
	return count
}

// getActiveCountAndStartIndex atomically returns the active provider count and
// the start index for iteration, avoiding TOCTOU race conditions.
func (cp *ClientProxy) getActiveCountAndStartIndex() (active int, startIndex int) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	now := time.Now()

	// Count active providers.
	for i := range cp.providers {
		if cp.deactivated[i].until.IsZero() || !now.Before(cp.deactivated[i].until) {
			active++
		}
	}
	if active == 0 || len(cp.providers) == 0 {
		return active, 0
	}

	// Ensure currentIndex is valid and points to an active provider.
	if cp.currentIndex < 0 || cp.currentIndex >= len(cp.providers) {
		cp.currentIndex = 0
	}
	if cp.deactivated[cp.currentIndex].until.IsZero() || !now.Before(cp.deactivated[cp.currentIndex].until) {
		return active, cp.currentIndex
	}
	cp.currentIndex = cp.nextActiveIndexLocked(cp.currentIndex)
	return active, cp.currentIndex
}

// getActiveCountAndCountTokensStartIndex atomically returns the active provider count and
// the start index for count_tokens iteration.
func (cp *ClientProxy) getActiveCountAndCountTokensStartIndex() (active int, startIndex int) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	now := time.Now()

	// Count active providers.
	for i := range cp.providers {
		if cp.deactivated[i].until.IsZero() || !now.Before(cp.deactivated[i].until) {
			active++
		}
	}
	if active == 0 || len(cp.providers) == 0 {
		return active, 0
	}

	// Ensure countTokensIndex is valid and points to an active provider.
	if cp.countTokensIndex < 0 || cp.countTokensIndex >= len(cp.providers) {
		cp.countTokensIndex = 0
	}
	if cp.deactivated[cp.countTokensIndex].until.IsZero() || !now.Before(cp.deactivated[cp.countTokensIndex].until) {
		return active, cp.countTokensIndex
	}
	cp.countTokensIndex = cp.nextActiveIndexLocked(cp.countTokensIndex)
	return active, cp.countTokensIndex
}

// handleAllUnavailable writes a Retry-After response if all providers are temporarily unavailable.
// Returns true if a response was written, false otherwise.
func (cp *ClientProxy) handleAllUnavailable(w http.ResponseWriter) bool {
	wait, reason, ok := cp.timeUntilNextAvailable()
	if !ok || wait <= 0 {
		return false
	}
	secs := int(wait/time.Second) + 1
	if secs < 1 {
		secs = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	if reason == "rate_limit" || reason == "overloaded" {
		http.Error(w, "All providers are rate limited; retry later", http.StatusTooManyRequests)
	} else {
		http.Error(w, "All providers are temporarily unavailable; retry later", http.StatusServiceUnavailable)
	}
	return true
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

func (cp *ClientProxy) allowCircuit(now time.Time, providerIndex int) circuitAllowResult {
	if providerIndex < 0 || providerIndex >= len(cp.breakers) {
		return circuitAllowResult{allowed: true}
	}
	cb := cp.breakers[providerIndex]
	if cb == nil {
		return circuitAllowResult{allowed: true}
	}
	return cb.allow(now)
}

func (cp *ClientProxy) releaseCircuitPermit(providerIndex int, usedProbe bool) {
	if providerIndex < 0 || providerIndex >= len(cp.breakers) {
		return
	}
	cb := cp.breakers[providerIndex]
	if cb == nil {
		return
	}
	cb.releaseProbeNeutral(usedProbe)
}

func shouldRecordCircuitFailure(reason string) bool {
	switch reason {
	case "network", "server", "idle_timeout":
		return true
	default:
		return false
	}
}

func (cp *ClientProxy) recordCircuitSuccess(now time.Time, providerIndex int, usedProbe bool) {
	if cp.mode == config.ClientModeManual {
		// Manual mode bypasses circuit breaker behavior and should not mutate breaker state.
		cp.releaseCircuitPermit(providerIndex, usedProbe)
		return
	}
	if providerIndex < 0 || providerIndex >= len(cp.breakers) {
		return
	}
	cb := cp.breakers[providerIndex]
	if cb == nil {
		return
	}
	cb.recordSuccess(now, usedProbe)
}

func (cp *ClientProxy) recordCircuitFailure(now time.Time, providerIndex int, usedProbe bool, reason string) {
	if cp.mode == config.ClientModeManual {
		// Manual mode bypasses circuit breaker behavior and should not mutate breaker state.
		cp.releaseCircuitPermit(providerIndex, usedProbe)
		return
	}
	if providerIndex < 0 || providerIndex >= len(cp.breakers) {
		return
	}
	cb := cp.breakers[providerIndex]
	if cb == nil {
		return
	}
	if shouldRecordCircuitFailure(reason) {
		cb.recordFailure(now, usedProbe)
	} else {
		cb.releaseProbeNeutral(usedProbe)
	}
}

func (cp *ClientProxy) recordCircuitFailureFromClassification(now time.Time, providerIndex int, usedProbe bool, reason string) {
	// Classification reasons map directly.
	cp.recordCircuitFailure(now, providerIndex, usedProbe, reason)
}

func (cp *ClientProxy) recordProviderSwitch(from string, to string, reason string, status int) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	cp.lastSwitch = ProviderSwitchEvent{
		At:     time.Now(),
		From:   from,
		To:     to,
		Reason: reason,
		Status: status,
	}
}
