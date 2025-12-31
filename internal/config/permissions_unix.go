//go:build !windows

package config

import (
	"fmt"
	"os"
)

func warnIfPermissiveConfig(path string) {
	// Check file permissions - warn if too permissive (world-readable).
	if fi, err := os.Stat(path); err == nil {
		mode := fi.Mode().Perm()
		// Warn if group or others have read permission (potential API key exposure).
		if mode&0o044 != 0 {
			// Using fmt.Fprintf since logger may not be initialized yet during config load.
			fmt.Fprintf(os.Stderr, "Warning: config file %s has permissive permissions (%o), consider chmod 600\n", path, mode)
		}
	}
}
