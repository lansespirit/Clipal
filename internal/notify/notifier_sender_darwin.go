//go:build darwin

package notify

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

var (
	pathEnvMu sync.Mutex

	terminalNotifierLookPath    = exec.LookPath
	commonTerminalNotifierPaths = []string{
		"/opt/homebrew/bin/terminal-notifier",
		"/usr/local/bin/terminal-notifier",
	}
)

func platformSender(sender func(title, message string, icon any) error) func(title, message string, icon any) error {
	if sender == nil {
		return nil
	}

	tnPath, err := resolveTerminalNotifierPath()
	if err != nil {
		return sender
	}

	dir := filepath.Dir(tnPath)
	if dir == "." || dir == "" {
		return sender
	}

	return func(title, message string, icon any) error {
		return withPATHDir(dir, func() error {
			return sender(title, message, icon)
		})
	}
}

func resolveTerminalNotifierPath() (string, error) {
	if p, err := terminalNotifierLookPath("terminal-notifier"); err == nil && strings.TrimSpace(p) != "" {
		return p, nil
	}

	for _, candidate := range commonTerminalNotifierPaths {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		info, err := os.Stat(candidate)
		if err != nil {
			continue
		}
		if info.IsDir() {
			continue
		}
		if info.Mode()&0o111 == 0 {
			continue
		}
		return candidate, nil
	}

	return "", exec.ErrNotFound
}

func withPATHDir(dir string, fn func() error) error {
	if strings.TrimSpace(dir) == "" {
		return fn()
	}

	pathEnvMu.Lock()
	defer pathEnvMu.Unlock()

	oldPath, hadPath := os.LookupEnv("PATH")
	if pathListContains(oldPath, dir) {
		return fn()
	}

	newPath := dir
	if hadPath && oldPath != "" {
		newPath = dir + string(os.PathListSeparator) + oldPath
	}
	if err := os.Setenv("PATH", newPath); err != nil {
		return err
	}
	defer func() {
		if hadPath {
			_ = os.Setenv("PATH", oldPath)
			return
		}
		_ = os.Unsetenv("PATH")
	}()

	return fn()
}

func pathListContains(pathValue string, wantDir string) bool {
	wantDir = strings.TrimSpace(wantDir)
	if wantDir == "" {
		return false
	}
	for _, entry := range filepath.SplitList(pathValue) {
		if strings.TrimSpace(entry) == wantDir {
			return true
		}
	}
	return false
}
