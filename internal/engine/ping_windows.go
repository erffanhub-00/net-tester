//go:build windows

package engine

import (
	"os/exec"
	"syscall"
)

// hidePingWindow hides the console window of a child ping process.
// The GUI itself runs without a console (-H windowsgui), but every
// ping it spawns would otherwise open a visible terminal window
// on Windows. This applies to both the batch ping and the
// continuous live ping.
func hidePingWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
