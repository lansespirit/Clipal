package logger

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRotatingFileWriter_RotatesAndCleansUp(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	w, err := NewRotatingFileWriter(dir, "clipal", 7)
	if err != nil {
		t.Fatalf("NewRotatingFileWriter: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })

	now := time.Date(2025, 12, 23, 12, 0, 0, 0, time.Local)
	w.localNowFunc = func() time.Time { return now }

	if _, err := w.Write([]byte("a\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	wantPath := filepath.Join(dir, "clipal-2025-12-23.log")
	if _, err := os.Stat(wantPath); err != nil {
		t.Fatalf("expected log file %s: %v", wantPath, err)
	}

	// Create an "old" file that should be removed (8 days ago).
	oldDay := now.AddDate(0, 0, -8).Format("2006-01-02")
	oldPath := filepath.Join(dir, "clipal-"+oldDay+".log")
	if err := os.WriteFile(oldPath, []byte("old\n"), 0o600); err != nil {
		t.Fatalf("WriteFile old: %v", err)
	}

	// Force rotation to the next day; this triggers cleanup.
	now = now.AddDate(0, 0, 1)
	if _, err := w.Write([]byte("b\n")); err != nil {
		t.Fatalf("Write rotate: %v", err)
	}

	if _, err := os.Stat(oldPath); err == nil {
		t.Fatalf("expected old log to be deleted: %s", oldPath)
	}

	nextPath := filepath.Join(dir, "clipal-2025-12-24.log")
	if _, err := os.Stat(nextPath); err != nil {
		t.Fatalf("expected rotated log file %s: %v", nextPath, err)
	}
}
