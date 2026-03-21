//go:build darwin

package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func getStatus(ctx context.Context, opts Options) (Status, string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Status{Manager: "launchd", Name: darwinLaunchdLabel}, "", err
	}
	plistPath := filepath.Join(home, "Library", "LaunchAgents", darwinLaunchdLabel+".plist")

	target := fmt.Sprintf("gui/%d", os.Getuid())
	serviceID := fmt.Sprintf("%s/%s", target, darwinLaunchdLabel)

	st := Status{
		Manager:    "launchd",
		Name:       darwinLaunchdLabel,
		Scope:      target,
		Installed:  fileExists(plistPath),
		BinaryPath: strings.TrimSpace(opts.BinaryPath),
		ConfigDir:  strings.TrimSpace(opts.ConfigDir),
	}

	cmd := exec.CommandContext(ctx, "launchctl", "print", serviceID)
	b, cmdErr := cmd.CombinedOutput()
	raw := strings.TrimSpace(string(b))

	// If not installed, treat missing/failed print as non-fatal: we still return Installed=false.
	if cmdErr != nil && !st.Installed {
		return st, raw, nil
	}
	// If installed but print fails, surface error for troubleshooting.
	if cmdErr != nil {
		return st, raw, cmdErr
	}

	return parseLaunchdStatus(st, raw), raw, nil
}
