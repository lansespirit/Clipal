//go:build !windows

package main

func maybeDetachConsole(detach bool) {
	_ = detach
}
