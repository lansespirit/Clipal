//go:build darwin

package notify

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestResolveTerminalNotifierPathFallsBackToCommonAbsolutePaths(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "terminal-notifier")
	if err := os.WriteFile(binPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake terminal-notifier: %v", err)
	}

	oldLookPath := terminalNotifierLookPath
	oldPaths := commonTerminalNotifierPaths
	terminalNotifierLookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	commonTerminalNotifierPaths = []string{binPath}
	defer func() {
		terminalNotifierLookPath = oldLookPath
		commonTerminalNotifierPaths = oldPaths
	}()

	got, err := resolveTerminalNotifierPath()
	if err != nil {
		t.Fatalf("resolveTerminalNotifierPath: %v", err)
	}
	if got != binPath {
		t.Fatalf("expected %q, got %q", binPath, got)
	}
}

func TestPlatformSenderPrependsDetectedTerminalNotifierDir(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "terminal-notifier")
	if err := os.WriteFile(binPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake terminal-notifier: %v", err)
	}

	oldLookPath := terminalNotifierLookPath
	oldPaths := commonTerminalNotifierPaths
	oldPathEnv, hadPathEnv := os.LookupEnv("PATH")
	terminalNotifierLookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	commonTerminalNotifierPaths = []string{binPath}
	if err := os.Setenv("PATH", "/usr/bin:/bin"); err != nil {
		t.Fatalf("set PATH: %v", err)
	}
	defer func() {
		terminalNotifierLookPath = oldLookPath
		commonTerminalNotifierPaths = oldPaths
		if hadPathEnv {
			_ = os.Setenv("PATH", oldPathEnv)
			return
		}
		_ = os.Unsetenv("PATH")
	}()

	called := false
	sender := platformSender(func(title, message string, icon any) error {
		called = true
		pathValue := os.Getenv("PATH")
		if !pathListContains(pathValue, tmpDir) {
			t.Fatalf("expected PATH %q to contain %q", pathValue, tmpDir)
		}
		return nil
	})
	if sender == nil {
		t.Fatalf("expected sender wrapper")
	}

	if err := sender("clipal", "test", nil); err != nil {
		t.Fatalf("sender returned error: %v", err)
	}
	if !called {
		t.Fatalf("expected wrapped sender to be called")
	}
	if got := os.Getenv("PATH"); got != "/usr/bin:/bin" {
		t.Fatalf("expected PATH to be restored, got %q", got)
	}
}

func TestPlatformSenderFallsBackWhenTerminalNotifierUnavailable(t *testing.T) {
	oldLookPath := terminalNotifierLookPath
	oldPaths := commonTerminalNotifierPaths
	terminalNotifierLookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	commonTerminalNotifierPaths = nil
	defer func() {
		terminalNotifierLookPath = oldLookPath
		commonTerminalNotifierPaths = oldPaths
	}()

	sentinel := errors.New("sentinel")
	sender := platformSender(func(title, message string, icon any) error {
		return sentinel
	})
	if sender == nil {
		t.Fatalf("expected sender wrapper")
	}
	if err := sender("clipal", "test", nil); !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
}
