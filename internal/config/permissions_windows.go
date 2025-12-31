//go:build windows

package config

func warnIfPermissiveConfig(path string) {
	// No-op on Windows: POSIX-style mode bits from os.Stat do not reflect NTFS ACLs,
	// and commonly appear as 0666 which would cause false warnings.
}
