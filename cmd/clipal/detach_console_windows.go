//go:build windows

package main

import "syscall"

func maybeDetachConsole(detach bool) {
	if !detach {
		return
	}
	// Best-effort: if we can't detach, keep running normally.
	_ = freeConsole()
}

func freeConsole() error {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	proc := kernel32.NewProc("FreeConsole")
	r1, _, e1 := proc.Call()
	if r1 != 0 {
		return nil
	}
	if e1 != syscall.Errno(0) {
		return e1
	}
	return syscall.EINVAL
}
