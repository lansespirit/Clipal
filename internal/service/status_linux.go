//go:build linux

package service

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

	// Parse key=value lines.
	var (
		loadState   string
		activeState string
		subState    string
		mainPID     int
	)
	for _, line := range strings.Split(raw, "\n") {
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch k {
		case "LoadState":
			loadState = v
		case "ActiveState":
			activeState = v
		case "SubState":
			subState = v
		case "MainPID":
			if pid, perr := strconv.Atoi(strings.TrimSpace(v)); perr == nil {
				mainPID = pid
			}
		}
	}

	st.Loaded = (loadState != "" && loadState != "not-found")
	st.Running = (activeState == "active")
	st.PID = mainPID
	if activeState != "" || subState != "" {
		st.Detail = "active=" + activeState + " sub=" + subState
	}

	return st, raw, nil
}
