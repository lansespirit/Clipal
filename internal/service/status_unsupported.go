//go:build !darwin && !linux && !windows

package service

import (
	"context"
)

func getStatus(ctx context.Context, opts Options) (Status, string, error) {
	_ = ctx
	_ = opts
	return Status{Manager: "unsupported", Name: "clipal", Scope: ""}, "", nil
}
