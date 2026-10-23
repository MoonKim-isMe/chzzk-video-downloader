//go:build !windows
package liverecording

import "os/exec"
func hideWindow(cmd *exec.Cmd){}
