//go:build !windows

package selfupdate

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func applyUnix(exePath string, newBinPath string) error {
	fi, err := os.Stat(exePath)
	if err != nil {
		return err
	}
	mode := fi.Mode()
	if mode&0o111 == 0 {
		mode |= 0o755
	}

	dir := filepath.Dir(exePath)
	tmp := filepath.Join(dir, fmt.Sprintf(".clipal-new-%d", time.Now().UnixNano()))
	if err := copyFile(tmp, newBinPath, mode.Perm()); err != nil {
		_ = os.Remove(tmp)
		return err
	}

	backup := exePath + ".old"
	_ = os.Remove(backup)

	if err := os.Rename(exePath, backup); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, exePath); err != nil {
		_ = os.Rename(backup, exePath)
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
