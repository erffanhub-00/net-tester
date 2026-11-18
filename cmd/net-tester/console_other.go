//go:build !windows

package main

// setupConsole is a no-op outside Windows: Unix terminals are
// UTF-8 and ANSI-capable by default.
func setupConsole() {}
