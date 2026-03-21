//go:build linux

package service

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func getStatus(ctx context.Context, opts Options) (Status, string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Status{Manager: "systemd", Name: linuxUnitName, Scope: "user"}, "", err
	}
	unitPath := filepath.Join(home, ".config", "systemd", "user", linuxUnitName)

	st := Status{
		Manager:    "systemd",
		Name:       linuxUnitName,
		Scope:      "user",
		Installed:  fileExists(unitPath),
		BinaryPath: strings.TrimSpace(opts.BinaryPath),
		ConfigDir:  strings.TrimSpace(opts.ConfigDir),
	}

	// Prefer `systemctl show` for stable machine-readable output.
	cmd := exec.CommandContext(ctx, "systemctl", "--user", "show", linuxUnitName,
		"--property=LoadState,ActiveState,SubState,MainPID,FragmentPath,ExecStart",
		"--no-page",
	)
	b, cmdErr := cmd.CombinedOutput()
	raw := strings.TrimSpace(string(b))

	if cmdErr != nil && !st.Installed {
		return st, raw, nil
	}
	if cmdErr != nil {
		return st, raw, cmdErr
	}

	return parseSystemdShowStatus(st, raw), raw, nil
}
