//go:build windows

package downloader

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"testing"
)

// Run on Windows to check the child's real console handle through each launch
// path, rather than only asserting the configured creation flags.
func TestWindowsToolProcessesHaveNoConsole(t *testing.T) {
	t.Setenv("CHZZK_TEST_CONSOLE_CHILD", "1")
	t.Run("runner", func(t *testing.T) {
		var output string
		err := NewRunner().Run(context.Background(), CommandSpec{Path: os.Args[0]}, func(line OutputLine) {
			if line.Stream == StreamStdout {
				output = line.Text
			}
		})
		if err != nil || output != "console:0" {
			t.Fatalf("background runner has a console: %q, error: %v", output, err)
		}
	})
	t.Run("version probe", func(t *testing.T) {
		output, err := probeToolVersion(context.Background(), os.Args[0], "--version")
		if err != nil || output != "console:0" {
			t.Fatalf("version probe has a console: %q, error: %v", output, err)
		}
	})
}

func init() {
	if os.Getenv("CHZZK_TEST_CONSOLE_CHILD") != "1" {
		return
	}
	getConsoleWindow := syscall.NewLazyDLL("kernel32.dll").NewProc("GetConsoleWindow")
	if err := getConsoleWindow.Find(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	window, _, _ := getConsoleWindow.Call()
	fmt.Fprintf(os.Stdout, "console:%d\n", window)
	os.Exit(0)
}
