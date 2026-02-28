//go:build windows

package service

import (
	"context"
	"os/exec"
	"regexp"
	"strings"
)

func getStatus(ctx context.Context, opts Options) (Status, string, error) {
	st := Status{
		Manager:    "schtasks",
		Name:       windowsTaskName,
		Scope:      "user",
		Installed:  true, // best-effort; refined below
		BinaryPath: strings.TrimSpace(opts.BinaryPath),
		ConfigDir:  strings.TrimSpace(opts.ConfigDir),
	}

	cmd := exec.CommandContext(ctx, "schtasks.exe", "/Query", "/TN", windowsTaskName, "/FO", "LIST", "/V")
	b, cmdErr := cmd.CombinedOutput()
	raw := strings.TrimSpace(string(b))
	if cmdErr != nil {
		// If the task doesn't exist, schtasks returns a non-zero exit code.
		st.Installed = false
		return st, raw, nil
	}

	st.Loaded = true

	// Best-effort parse: "Status: Ready|Running|Disabled|..."
	re := regexp.MustCompile(`(?mi)^\s*Status:\s*(.+?)\s*$`)
	if m := re.FindStringSubmatch(raw); len(m) == 2 {
		status := strings.TrimSpace(m[1])
		st.Detail = "status=" + status
		if strings.EqualFold(status, "Running") {
			st.Running = true
		}
	}

	return st, raw, nil
}
