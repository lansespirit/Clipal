package proxy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/lansespirit/Clipal/internal/logger"
)

// streamResponseToClient handles the final stage of an upstream attempt: waiting for the first byte,
// committing headers, and streaming the body. It handles idle timeouts, circuit breaker recording,
// and cleanup. It returns true if it committed headers (indicating the attempt is final), and false
// if it failed before headers (allowing the caller to potentially fail over).
func (cp *ClientProxy) streamResponseToClient(w http.ResponseWriter, resp *http.Response, originalReq *http.Request, attemptCtx context.Context, cancelAttempt context.CancelCauseFunc, index int, allow circuitAllowResult, onCommit func()) bool {
	// Stream response to the client, with idle-timeout protection.
	var idleTimer *time.Timer
	if cp.upstreamIdle > 0 {
		idleTimer = time.AfterFunc(cp.upstreamIdle, func() { cancelAttempt(errUpstreamIdleTimeout) })
	}

	buf := make([]byte, 32*1024)
	total := 0
	firstN, firstErr := resp.Body.Read(buf)
	if firstN > 0 && idleTimer != nil {
		idleTimer.Reset(cp.upstreamIdle)
	}
	total += firstN

	if firstN == 0 && firstErr != nil {
		resp.Body.Close()
		stopTimer(idleTimer)
		if errors.Is(firstErr, io.EOF) {
			// Legitimately empty body; pass through as-is.
			if onCommit != nil {
				onCommit()
			}
			copyHeaders(w.Header(), resp.Header)
			w.WriteHeader(resp.StatusCode)
			cp.recordCircuitSuccess(time.Now(), index, allow.usedProbe)
			cancelAttempt(nil)
			return true
		}

		if originalReq.Context().Err() != nil {
			// Client went away; do not record a provider failure.
			cp.releaseCircuitPermit(index, allow.usedProbe)
			cancelAttempt(nil)
			return true
		}
		// Return false so the caller can handle failure (e.g. failover).
		return false
	}

	// Committed to this provider.
	if onCommit != nil {
		onCommit()
	}
	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

	fw := newFlushWriter(w)
	if firstN > 0 {
		if _, err := fw.Write(buf[:firstN]); err != nil {
			resp.Body.Close()
			stopTimer(idleTimer)
			cp.releaseCircuitPermit(index, allow.usedProbe)
			cancelAttempt(nil)
			return true
		}
	}

	var copyErr error
	for {
		nr, er := resp.Body.Read(buf)
		if nr > 0 {
			if idleTimer != nil {
				idleTimer.Reset(cp.upstreamIdle)
			}
			total += nr
			if _, ew := fw.Write(buf[:nr]); ew != nil {
				resp.Body.Close()
				stopTimer(idleTimer)
				cp.releaseCircuitPermit(index, allow.usedProbe)
				cancelAttempt(nil)
				return true
			}
		}
		if er != nil {
			if errors.Is(er, io.EOF) {
				break
			}
			copyErr = er
			if originalReq.Context().Err() == nil {
				if isUpstreamIdleTimeout(attemptCtx, er) {
					logger.Warn("[%s] upstream %s stalled for %s (after %d bytes)", cp.clientType, cp.providers[index].Name, cp.upstreamIdle, total)
				} else {
					logger.Warn("[%s] response copy failed via %s: %v", cp.clientType, cp.providers[index].Name, er)
				}
			}
			break
		}
	}

	resp.Body.Close()
	stopTimer(idleTimer)
	if copyErr != nil && originalReq.Context().Err() != nil {
		cp.releaseCircuitPermit(index, allow.usedProbe)
		cancelAttempt(nil)
		return true
	}
	if copyErr == nil {
		cp.recordCircuitSuccess(time.Now(), index, allow.usedProbe)
	} else if isUpstreamIdleTimeout(attemptCtx, copyErr) {
		cp.recordCircuitFailure(time.Now(), index, allow.usedProbe, "idle_timeout")
	} else {
		cp.recordCircuitFailure(time.Now(), index, allow.usedProbe, "network")
	}
	cancelAttempt(nil)
	return true
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
