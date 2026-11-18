//go:build !windows

package engine

import "os/exec"

// hidePingWindow is a no-op outside Windows: only a Windows child
// process opens a visible console window.
func hidePingWindow(cmd *exec.Cmd) {}
