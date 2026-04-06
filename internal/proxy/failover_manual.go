package proxy

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/lansespirit/Clipal/internal/logger"
)

func (cp *ClientProxy) forwardManual(w http.ResponseWriter, req *http.Request, path string) {
	if err := req.Context().Err(); err != nil {
		return
	}

	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		logger.Error("[%s] failed to read request body: %v", cp.clientType, err)
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			cp.recordTerminalRequest(time.Now(), req, "", http.StatusRequestEntityTooLarge, "request_rejected", "Request body too large.")
			writeProxyError(w, "Request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		cp.recordTerminalRequest(time.Now(), req, "", http.StatusBadRequest, "request_rejected", "Failed to read request body.")
		writeProxyError(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer func() { _ = req.Body.Close() }()

	index := cp.pinnedIndex
	if index < 0 && cp.pinnedProvider != "" {
		// Defensive fallback: pinnedIndex should be resolved during initialization.
		// If we reach here, it may indicate a reload race or config mismatch.
		logger.Warn("[%s] pinnedIndex not resolved at init; falling back to name lookup for %q", cp.clientType, cp.pinnedProvider)
		for i := range cp.providers {
			if cp.providers[i].Name == cp.pinnedProvider {
				index = i
				break
			}
		}
	}
	if index < 0 || index >= len(cp.providers) {
		logger.Warn("[%s] manual mode enabled but pinned provider not found", cp.clientType)
		cp.recordTerminalRequest(time.Now(), req, "", http.StatusServiceUnavailable, "all_providers_unavailable", "Pinned provider not configured.")
		writeProxyError(w, "Pinned provider not configured", http.StatusServiceUnavailable)
		return
	}

	provider := cp.providers[index]
	scope := routingScopeForRequest(req)
	if index < 0 || index >= len(cp.providerKeys) || len(cp.providerKeys[index]) == 0 {
		cp.recordTerminalRequest(time.Now(), req, provider.Name, http.StatusServiceUnavailable, "all_providers_unavailable", "Pinned provider has no configured API keys.")
		writeProxyError(w, "Pinned provider has no configured API keys", http.StatusServiceUnavailable)
		return
	}
	keyIndex := cp.preferredKeyIndexForScope(index, scope)
	attemptCtx, cancelAttempt := context.WithCancelCause(req.Context())
	reqWithAttemptCtx := req.WithContext(attemptCtx)
	proxyReq, err := cp.createProxyRequest(reqWithAttemptCtx, provider, cp.providerKeys[index][keyIndex], path, bodyBytes)
	if err != nil {
		logger.Error("[%s] failed to create request for %s: %v", cp.clientType, provider.Name, err)
		cancelAttempt(nil)
		cp.recordTerminalRequest(time.Now(), req, provider.Name, http.StatusBadGateway, "request_rejected", "Failed to create upstream request.")
		writeProxyError(w, "Failed to create upstream request", http.StatusBadGateway)
		return
	}

	proxyReq.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(bodyBytes)), nil
	}

	//nolint:gosec // Clipal is a user-configured reverse proxy and intentionally forwards to the pinned upstream provider.
	resp, err := cp.upstreamHTTPClient(index).Do(proxyReq)
	if err != nil {
		if req.Context().Err() != nil {
			cancelAttempt(nil)
			return
		}
		cancelAttempt(nil)
		cp.recordTerminalRequest(time.Now(), req, provider.Name, http.StatusBadGateway, "failed_before_response", describeAttemptFailure(provider.Name, "network", 0, true)+".")
		writeProxyError(w, "Upstream request failed", http.StatusBadGateway)
		return
	}

	onCommit := func() {
		cp.setCurrentIndexForScope(index, scope)
		cp.setCurrentKeyIndexForScope(index, keyIndex, scope)
	}
	onSuccess := func(success streamSuccess) {
		cp.recordCompletedUsage(req, provider.Name, resp.StatusCode, success.usage, time.Now())
	}
	allow := circuitAllowResult{}
	result := cp.streamResponseToClient(w, resp, req, attemptCtx, cancelAttempt, index, allow, onCommit, onSuccess)
	if result.kind == streamFinal {
		cp.logRequestResult(req, provider.Name, resp.StatusCode, result, true)
		return
	}

	cancelAttempt(nil)
	reason := "network"
	if isUpstreamIdleTimeout(attemptCtx, attemptCtx.Err()) {
		reason = "idle_timeout"
	}
	cp.recordTerminalRequest(time.Now(), req, provider.Name, http.StatusBadGateway, "failed_before_response", describeAttemptFailure(provider.Name, reason, 0, true)+".")
	writeProxyError(w, "Upstream response read failed", http.StatusBadGateway)
}
