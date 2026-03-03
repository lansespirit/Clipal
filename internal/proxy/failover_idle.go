package proxy

import (
	"context"
	"errors"
	"time"
)

var errUpstreamIdleTimeout = errors.New("upstream idle timeout")

func isUpstreamIdleTimeout(ctx context.Context, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errUpstreamIdleTimeout) {
		return true
	}
	if errors.Is(err, context.Canceled) && context.Cause(ctx) == errUpstreamIdleTimeout {
		return true
	}
	return false
}

func stopTimer(t *time.Timer) {
	if t == nil {
		return
	}
	t.Stop()
}
