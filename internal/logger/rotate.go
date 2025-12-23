package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type RotatingFileWriter struct {
	dir           string
	prefix        string
	retentionDays int

	mu           sync.Mutex
	currentDay   string
	currentFile  *os.File
	cleanupDay   string
	localNowFunc func() time.Time
}

func NewRotatingFileWriter(dir, prefix string, retentionDays int) (*RotatingFileWriter, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, fmt.Errorf("log dir is empty")
	}
	if retentionDays <= 0 {
		retentionDays = 7
	}
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "clipal"
	}

	w := &RotatingFileWriter{
		dir:           dir,
		prefix:        prefix,
		retentionDays: retentionDays,
		localNowFunc:  time.Now,
	}

	if err := os.MkdirAll(w.dir, 0o755); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *RotatingFileWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := w.localNowFunc()
	day := now.Format("2006-01-02")
	if err := w.ensureOpenLocked(day); err != nil {
		return 0, err
	}
	return w.currentFile.Write(p)
}

func (w *RotatingFileWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.currentFile == nil {
		return nil
	}
	err := w.currentFile.Close()
	w.currentFile = nil
	w.currentDay = ""
	return err
}

func (w *RotatingFileWriter) ensureOpenLocked(day string) error {
	if w.currentFile != nil && w.currentDay == day {
		return nil
	}

	if w.currentFile != nil {
		_ = w.currentFile.Close()
		w.currentFile = nil
	}

	if err := os.MkdirAll(w.dir, 0o755); err != nil {
		return err
	}

	path := filepath.Join(w.dir, fmt.Sprintf("%s-%s.log", w.prefix, day))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	w.currentFile = f
	w.currentDay = day

	// Cleanup at most once per day.
	if w.cleanupDay != day {
		_ = w.cleanupLocked(day)
		w.cleanupDay = day
	}
	return nil
}

func (w *RotatingFileWriter) cleanupLocked(today string) error {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return err
	}

	type candidate struct {
		path string
		day  string
	}

	var cands []candidate
	wantPrefix := w.prefix + "-"
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, wantPrefix) || !strings.HasSuffix(name, ".log") {
			continue
		}
		day := strings.TrimSuffix(strings.TrimPrefix(name, wantPrefix), ".log")
		// Expect YYYY-MM-DD exactly.
		if len(day) != len("2006-01-02") {
			continue
		}
		if _, err := time.Parse("2006-01-02", day); err != nil {
			continue
		}
		cands = append(cands, candidate{path: filepath.Join(w.dir, name), day: day})
	}

	sort.Slice(cands, func(i, j int) bool { return cands[i].day < cands[j].day })

	cutoff := w.localNowFunc().AddDate(0, 0, -w.retentionDays).Format("2006-01-02")
	for _, c := range cands {
		if c.day >= cutoff {
			continue
		}
		_ = os.Remove(c.path)
	}
	return nil
}

func multiWriter(writers ...io.Writer) io.Writer {
	var out []io.Writer
	for _, w := range writers {
		if w != nil {
			out = append(out, w)
		}
	}
	if len(out) == 0 {
		return io.Discard
	}
	if len(out) == 1 {
		return out[0]
	}
	return io.MultiWriter(out...)
}
