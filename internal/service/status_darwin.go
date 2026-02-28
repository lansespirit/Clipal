//go:build darwin

package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
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

	st.Loaded = true

	// Parse a few useful fields (best-effort; launchctl output format isn't a strict API).
	if m := regexp.MustCompile(`(?m)^\s*state\s*=\s*(\S+)`).FindStringSubmatch(raw); len(m) == 2 {
		st.Detail = "state=" + m[1]
		if m[1] == "running" {
			st.Running = true
		}
	}
	if m := regexp.MustCompile(`(?m)^\s*pid\s*=\s*(\d+)`).FindStringSubmatch(raw); len(m) == 2 {
		if pid, perr := strconv.Atoi(m[1]); perr == nil {
			st.PID = pid
			if pid > 0 {
				st.Running = true
			}
		}
	}
	if m := regexp.MustCompile(`(?m)^\s*program\s*=\s*(.+)$`).FindStringSubmatch(raw); len(m) == 2 {
		st.BinaryPath = strings.TrimSpace(m[1])
	}
	if m := regexp.MustCompile(`(?m)^\s*stdout path\s*=\s*(.+)$`).FindStringSubmatch(raw); len(m) == 2 {
		st.StdoutPath = strings.TrimSpace(m[1])
	}
	if m := regexp.MustCompile(`(?m)^\s*stderr path\s*=\s*(.+)$`).FindStringSubmatch(raw); len(m) == 2 {
		st.StderrPath = strings.TrimSpace(m[1])
	}
	if m := regexp.MustCompile(`(?m)^\s*last exit reason\s*=\s*(.+)$`).FindStringSubmatch(raw); len(m) == 2 {
		st.LastExit = strings.TrimSpace(m[1])
	}

	return st, raw, nil
}
