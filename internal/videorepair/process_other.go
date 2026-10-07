//go:build !windows

package videorepair

import "os/exec"

func hideConsoleWindow(cmd *exec.Cmd) {}
