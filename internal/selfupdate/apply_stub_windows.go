//go:build windows

package selfupdate

import "fmt"

func applyUnix(exePath string, newBinPath string) error {
	return fmt.Errorf("unix self-update not supported on this platform")
}
