//go:build !windows

package engine

import (
	"os/exec"
	"testing"
)

// TestHidePingWindowNoOp checks the non-Windows stub leaves the
// command alone: only a Windows child process opens a visible
// console window.
func TestHidePingWindowNoOp(t *testing.T) {
	cmd := exec.Command("ping", "-c", "1", "127.0.0.1")
	hidePingWindow(cmd)
	if cmd.SysProcAttr != nil {
		t.Fatal("hidePingWindow set SysProcAttr on a non-Windows build")
	}
}
