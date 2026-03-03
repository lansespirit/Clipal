package proxy

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"

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
			writeProxyError(w, "Request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		writeProxyError(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

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
		writeProxyError(w, "Pinned provider not configured", http.StatusServiceUnavailable)
		return
	}

	provider := cp.providers[index]
	attemptCtx, cancelAttempt := context.WithCancelCause(req.Context())

	reqWithAttemptCtx := req.WithContext(attemptCtx)
	proxyReq, err := cp.createProxyRequest(reqWithAttemptCtx, provider, path, bodyBytes)
	if err != nil {
		logger.Error("[%s] failed to create request for %s: %v", cp.clientType, provider.Name, err)
		cancelAttempt(nil)
		writeProxyError(w, "Failed to create upstream request", http.StatusBadGateway)
		return
	}

	proxyReq.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(bodyBytes)), nil
	}

	resp, err := cp.httpClient.Do(proxyReq)
	if err != nil {
		if req.Context().Err() != nil {
			cancelAttempt(nil)
			return
		}
		cancelAttempt(nil)
		writeProxyError(w, "Upstream request failed", http.StatusBadGateway)
		return
	}

	// Manual mode: always return the pinned provider's response (no failover, no cooldown,
	// and no circuit breaker state changes).

	onCommit := func() { cp.setCurrentIndex(index) }
	allow := circuitAllowResult{}
	if committed := cp.streamResponseToClient(w, resp, req, attemptCtx, cancelAttempt, index, allow, onCommit); committed {
		if resp.StatusCode == http.StatusOK {
			logger.Info("[%s] request completed via %s (manual)", cp.clientType, provider.Name)
		}
		return
	}

	cancelAttempt(nil)
	writeProxyError(w, "Upstream response read failed", http.StatusBadGateway)
}
