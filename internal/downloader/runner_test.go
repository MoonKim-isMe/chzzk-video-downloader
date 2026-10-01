package downloader

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestRunnerStreamsOutput(t *testing.T) {
	spec := helperCommand("success")
	var lines []OutputLine
	err := NewRunner().Run(context.Background(), spec, func(line OutputLine) {
		lines = append(lines, line)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %#v", lines)
	}
	foundStdout, foundStderr := false, false
	for _, line := range lines {
		foundStdout = foundStdout || (line.Stream == StreamStdout && line.Text == "out-line")
		foundStderr = foundStderr || (line.Stream == StreamStderr && line.Text == "err-line")
	}
	if !foundStdout || !foundStderr {
		t.Fatalf("unexpected output: %#v", lines)
	}
}

func TestRunnerReturnsProcessError(t *testing.T) {
	err := NewRunner().Run(context.Background(), helperCommand("fail"), nil)
	var processErr *ProcessError
	if !errors.As(err, &processErr) {
		t.Fatalf("expected ProcessError, got %T: %v", err, err)
	}
	if processErr.ExitCode != 7 || !strings.Contains(processErr.Error(), "failed-line") {
		t.Fatalf("unexpected process error: %#v", processErr)
	}
}

func helperCommand(mode string) CommandSpec {
	return CommandSpec{
		Path: os.Args[0],
		Args: []string{"-test.run=TestRunnerHelperProcess", "--", mode},
		Env:  []string{"GO_WANT_HELPER_PROCESS=1"},
	}
}

func TestRunnerHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
}

func init() {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	mode := os.Args[len(os.Args)-1]
	switch mode {
	case "success":
		fmt.Fprintln(os.Stdout, "out-line")
		fmt.Fprintln(os.Stderr, "err-line")
		os.Exit(0)
	case "fail":
		fmt.Fprintln(os.Stderr, "failed-line")
		os.Exit(7)
	default:
		os.Exit(2)
	}
}
