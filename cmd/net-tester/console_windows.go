//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

// setupConsole makes Windows output UTF-8 and enables ANSI escape
// processing, so the program behaves the same on cmd.exe,
// PowerShell and Windows Terminal. Every call is best-effort: if
// the console does not support it, output simply stays plain.
// (Windows defaults to a legacy code page such as cp1252, which
// garbles non-ASCII output — the KeyLayoutFixer lesson.)
func setupConsole() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	setCP := kernel32.NewProc("SetConsoleOutputCP")
	setCP.Call(uintptr(65001)) // UTF-8

	getStdHandle := kernel32.NewProc("GetStdHandle")
	getMode := kernel32.NewProc("GetConsoleMode")
	setMode := kernel32.NewProc("SetConsoleMode")
	const stdOutputHandle = ^uintptr(10) // -11 as uintptr
	h, _, _ := getStdHandle.Call(stdOutputHandle)
	if h == 0 || h == ^uintptr(0) {
		return
	}
	var mode uint32
	ret, _, _ := getMode.Call(h, uintptr(unsafe.Pointer(&mode)))
	if ret == 0 {
		return
	}
	const enableVirtualTerminalProcessing = 0x0004
	setMode.Call(h, uintptr(mode|enableVirtualTerminalProcessing))
}
