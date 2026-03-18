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
		detail := reactivationDetail(d.reason)
		cp.deactivated[i] = providerDeactivation{}
		if i < len(cp.providers) {
			logger.Info("[%s] provider %s %s", cp.clientType, cp.providers[i].Name, detail)
		} else {
			logger.Info("[%s] provider #%d %s", cp.clientType, i, detail)
		}
	}
	for i := range cp.keyDeactivated {
		for j, d := range cp.keyDeactivated[i] {
			if d.until.IsZero() {
				continue
			}
			if now.Before(d.until) {
				continue
			}
			cp.keyDeactivated[i][j] = providerDeactivation{}
			if i < len(cp.providers) {
				logger.Info("[%s] provider %s key %d/%d reactivated", cp.clientType, cp.providers[i].Name, j+1, len(cp.providerKeys[i]))
			}
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

func (cp *ClientProxy) isKeyDeactivated(providerIndex int, keyIndex int) bool {
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	return cp.isKeyDeactivatedLocked(providerIndex, keyIndex, time.Now())
}

func (cp *ClientProxy) isKeyDeactivatedLocked(providerIndex int, keyIndex int, now time.Time) bool {
	if providerIndex < 0 || providerIndex >= len(cp.keyDeactivated) {
		return false
	}
	if keyIndex < 0 || keyIndex >= len(cp.keyDeactivated[providerIndex]) {
		return false
	}
	d := cp.keyDeactivated[providerIndex][keyIndex]
	return !d.until.IsZero() && now.Before(d.until)
}

func (cp *ClientProxy) availableKeyCountLocked(providerIndex int, now time.Time) int {
	if providerIndex < 0 || providerIndex >= len(cp.providerKeys) {
		return 0
	}
	count := 0
	for keyIndex := range cp.providerKeys[providerIndex] {
		if !cp.isKeyDeactivatedLocked(providerIndex, keyIndex, now) {
			count++
		}
	}
	return count
}

func (cp *ClientProxy) providerAvailableForRoutingLocked(providerIndex int, now time.Time) bool {
	if providerIndex < 0 || providerIndex >= len(cp.providers) {
		return false
	}
	if !cp.deactivated[providerIndex].until.IsZero() && now.Before(cp.deactivated[providerIndex].until) {
		return false
	}
	if cp.availableKeyCountLocked(providerIndex, now) == 0 {
		return false
	}
	if providerIndex < len(cp.breakers) && cp.breakers[providerIndex] != nil {
		allow := cp.breakers[providerIndex].peekAllow(now)
		if !allow.allowed && allow.wait > 0 {
			return false
		}
	}
	return true
}

func (cp *ClientProxy) nextActiveKeyIndexLocked(providerIndex int, from int, now time.Time) int {
	if providerIndex < 0 || providerIndex >= len(cp.providerKeys) {
		return 0
	}
	n := len(cp.providerKeys[providerIndex])
	if n == 0 {
		return 0
	}
	if from < 0 || from >= n {
		from = 0
	}
	if !cp.isKeyDeactivatedLocked(providerIndex, from, now) {
		return from
	}
	for step := 1; step <= n; step++ {
		idx := (from + step) % n
		if !cp.isKeyDeactivatedLocked(providerIndex, idx, now) {
			return idx
		}
	}
	return from
}

func (cp *ClientProxy) ensureActiveKeyIndexLocked(providerIndex int, countTokens bool, now time.Time) int {
	if providerIndex < 0 || providerIndex >= len(cp.providerKeys) || len(cp.providerKeys[providerIndex]) == 0 {
		return 0
	}
	cur := cp.currentKeyIndex
	if countTokens {
		cur = cp.countTokensKeyIndex
	}
	if providerIndex >= len(cur) {
		return 0
	}
	if cur[providerIndex] < 0 || cur[providerIndex] >= len(cp.providerKeys[providerIndex]) {
		cur[providerIndex] = 0
	}
	cur[providerIndex] = cp.nextActiveKeyIndexLocked(providerIndex, cur[providerIndex], now)
	return cur[providerIndex]
}

func (cp *ClientProxy) setCurrentKeyIndex(providerIndex int, keyIndex int) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	if providerIndex < 0 || providerIndex >= len(cp.currentKeyIndex) {
		return
	}
	cp.currentKeyIndex[providerIndex] = keyIndex
}

func (cp *ClientProxy) setCountTokensKeyIndex(providerIndex int, keyIndex int) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	if providerIndex < 0 || providerIndex >= len(cp.countTokensKeyIndex) {
		return
	}
	cp.countTokensKeyIndex[providerIndex] = keyIndex
}

func (cp *ClientProxy) activeKeyCount(providerIndex int) int {
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	return cp.availableKeyCountLocked(providerIndex, time.Now())
}

func (cp *ClientProxy) nextActiveKeyIndex(providerIndex int, from int) int {
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	return cp.nextActiveKeyIndexLocked(providerIndex, from, time.Now())
}

func (cp *ClientProxy) preferredKeyIndex(providerIndex int, countTokens bool) int {
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	if providerIndex < 0 || providerIndex >= len(cp.providerKeys) || len(cp.providerKeys[providerIndex]) == 0 {
		return 0
	}
	cur := cp.currentKeyIndex
	if countTokens {
		cur = cp.countTokensKeyIndex
	}
	if providerIndex >= len(cur) {
		return 0
	}
	if cur[providerIndex] < 0 || cur[providerIndex] >= len(cp.providerKeys[providerIndex]) {
		return 0
	}
	return cur[providerIndex]
}

func (cp *ClientProxy) getActiveKeyCountAndStartIndex(providerIndex int, countTokens bool) (active int, startIndex int) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	now := time.Now()
	if providerIndex < 0 || providerIndex >= len(cp.providerKeys) || len(cp.providerKeys[providerIndex]) == 0 {
		return 0, 0
	}
	active = cp.availableKeyCountLocked(providerIndex, now)
	if active == 0 {
		return 0, 0
	}
	return active, cp.ensureActiveKeyIndexLocked(providerIndex, countTokens, now)
}

func (cp *ClientProxy) deactivateKeyFor(providerIndex int, keyIndex int, reason string, status int, msg string, d time.Duration) {
	if d <= 0 {
		return
	}
	cp.mu.Lock()
	defer cp.mu.Unlock()
	if providerIndex < 0 || providerIndex >= len(cp.keyDeactivated) {
		return
	}
	if keyIndex < 0 || keyIndex >= len(cp.keyDeactivated[providerIndex]) {
		return
	}
	now := time.Now()
	if !cp.keyDeactivated[providerIndex][keyIndex].until.IsZero() && now.Before(cp.keyDeactivated[providerIndex][keyIndex].until) {
		return
	}
	cp.keyDeactivated[providerIndex][keyIndex] = providerDeactivation{
		at:      now,
		until:   now.Add(d),
		reason:  reason,
		status:  status,
		message: msg,
	}
	if providerIndex < len(cp.currentKeyIndex) && cp.currentKeyIndex[providerIndex] == keyIndex {
		cp.currentKeyIndex[providerIndex] = cp.nextActiveKeyIndexLocked(providerIndex, keyIndex, now)
	}
	if providerIndex < len(cp.countTokensKeyIndex) && cp.countTokensKeyIndex[providerIndex] == keyIndex {
		cp.countTokensKeyIndex[providerIndex] = cp.nextActiveKeyIndexLocked(providerIndex, keyIndex, now)
	}
}

func (cp *ClientProxy) timeUntilNextKeyAvailableLocked(providerIndex int, now time.Time) (wait time.Duration, reason string, ok bool) {
	if providerIndex < 0 || providerIndex >= len(cp.keyDeactivated) {
		return 0, "", false
	}
	var soonest time.Time
	var soonestReason string
	for _, d := range cp.keyDeactivated[providerIndex] {
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
	if keyWait, keyReason, keyOK := func() (time.Duration, string, bool) {
		cp.mu.RLock()
		defer cp.mu.RUnlock()
		if cp.availableKeyCountLocked(index, now) > 0 {
			return 0, "", false
		}
		return cp.timeUntilNextKeyAvailableLocked(index, now)
	}(); keyOK {
		return keyWait, keyReason, true
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
	for i := range cp.providers {
		if cp.availableKeyCountLocked(i, now) > 0 {
			continue
		}
		wait, reason, ok := cp.timeUntilNextKeyAvailableLocked(i, now)
		if !ok || wait <= 0 {
			continue
		}
		until := now.Add(wait)
		if soonest.IsZero() || until.Before(soonest) {
			soonest = until
			soonestReason = reason
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
		if cp.providerAvailableForRoutingLocked(i, now) {
			count++
		}
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
		if cp.providerAvailableForRoutingLocked(i, now) {
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
	if cp.providerAvailableForRoutingLocked(cp.currentIndex, now) {
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
		if cp.providerAvailableForRoutingLocked(i, now) {
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
	if cp.providerAvailableForRoutingLocked(cp.countTokensIndex, now) {
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
	if cp.providerAvailableForRoutingLocked(cp.currentIndex, now) {
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
	if cp.providerAvailableForRoutingLocked(cp.countTokensIndex, now) {
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
		if cp.providerAvailableForRoutingLocked(idx, now) {
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

func (cp *ClientProxy) recordLastRequest(now time.Time, provider string, status int, result streamResult) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	cp.lastRequest = RequestOutcomeEvent{
		At:       now,
		Provider: provider,
		Status:   status,
		Delivery: string(result.delivery),
		Protocol: string(result.protocol),
		Cause:    result.cause,
		Bytes:    result.bytes,
	}
}

func (cp *ClientProxy) recordTerminalRequest(now time.Time, provider string, status int, result string, detail string) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	cp.lastRequest = RequestOutcomeEvent{
		At:       now,
		Provider: provider,
		Status:   status,
		Result:   result,
		Detail:   detail,
	}
}
