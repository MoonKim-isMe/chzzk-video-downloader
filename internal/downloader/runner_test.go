package downloader

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
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

func TestRunnerSplitsCarriageReturnProgress(t *testing.T) {
	var lines []OutputLine
	err := NewRunner().Run(context.Background(), helperCommand("carriage"), func(line OutputLine) {
		lines = append(lines, line)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 ||
		lines[0].Text != "frame=1" ||
		lines[1].Text != "frame=2" {
		t.Fatalf("unexpected carriage-return output: %#v", lines)
	}
}

func TestRunnerCancellationStopsProcess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	started := time.Now()
	err := NewRunner().Run(ctx, helperCommand("wait"), func(line OutputLine) {
		if line.Stream == StreamStdout && line.Text == "waiting" {
			cancel()
		}
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
	if time.Since(started) > 2*time.Second {
		t.Fatalf("cancelled process did not exit promptly: %s", time.Since(started))
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
	case "carriage":
		fmt.Fprint(os.Stderr, "frame=1\rframe=2\r")
		os.Exit(0)
	case "wait":
		fmt.Fprintln(os.Stdout, "waiting")
		for {
			time.Sleep(time.Second)
		}
	default:
		os.Exit(2)
	}
}


func TestMergeCommandEnvOverridesCaseInsensitive(t *testing.T) {
	base := []string{
		"Path=C:\\Windows\\System32",
		"KEEP=value",
	}
	merged := mergeCommandEnv(base, []string{
		"PATH=C:\\Tools;C:\\Windows\\System32",
		"NEW=value",
	})

	pathCount := 0
	for _, entry := range merged {
		if strings.EqualFold(commandEnvName(entry), "PATH") {
			pathCount++
			if entry != "PATH=C:\\Tools;C:\\Windows\\System32" {
				t.Fatalf("unexpected PATH override: %q", entry)
			}
		}
	}
	if pathCount != 1 {
		t.Fatalf("expected one PATH entry, got %d in %#v", pathCount, merged)
	}
	if !slices.Contains(merged, "KEEP=value") || !slices.Contains(merged, "NEW=value") {
		t.Fatalf("environment merge lost entries: %#v", merged)
	}
}

func commandEnvName(entry string) string {
	index := strings.IndexByte(entry, '=')
	if index <= 0 {
		return entry
	}
	return entry[:index]
}
