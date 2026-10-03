//go:build windows

package downloader

import (
	"os/exec"
	"syscall"
)

// CREATE_NO_WINDOW prevents console allocation for background tools while their
// stdout/stderr pipes remain available for progress and error reporting.
const createNoWindow = 0x08000000

func hideConsoleWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= createNoWindow
}
