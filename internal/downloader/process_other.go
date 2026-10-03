//go:build !windows

package downloader

import "os/exec"

func hideConsoleWindow(cmd *exec.Cmd) {}
