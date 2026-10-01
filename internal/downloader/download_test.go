package downloader

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func downloadHelperCommand(mode string) CommandSpec {
	return CommandSpec{
		Path: os.Args[0],
		Args: []string{"-test.run=TestDownloadHelperProcess", "--", mode},
		Env:  []string{"GO_WANT_DOWNLOAD_HELPER_PROCESS=1"},
	}
}

func TestRunPreparedDownload(t *testing.T) {
	manager := &Manager{runner: NewRunner()}
	var progressEvents []DownloadProgress

	result, err := manager.runPreparedDownload(
		context.Background(),
		downloadHelperCommand("success"),
		func(progress DownloadProgress) {
			progressEvents = append(progressEvents, progress)
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.FinalPath != `C:\\Video\\done.mp4` {
		t.Fatalf("unexpected final path: %q", result.FinalPath)
	}
	if result.LastProgress.Status != "completed" || result.LastProgress.Percent != 100 {
		t.Fatalf("unexpected last progress: %#v", result.LastProgress)
	}
	if len(progressEvents) != 3 || progressEvents[len(progressEvents)-1].Status != "completed" {
		t.Fatalf("unexpected progress events: %#v", progressEvents)
	}
}

func TestRunPreparedDownloadRequiresFinalPath(t *testing.T) {
	manager := &Manager{runner: NewRunner()}
	_, err := manager.runPreparedDownload(context.Background(), downloadHelperCommand("no-file"), nil)
	if err == nil || !strings.Contains(err.Error(), "최종 파일 경로") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunPreparedDownloadSupportsContextCancellation(t *testing.T) {
	manager := &Manager{runner: NewRunner()}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := manager.runPreparedDownload(ctx, downloadHelperCommand("wait"), nil)
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unexpected cancellation error: %v", err)
	}
}

func TestRunPreparedDownloadSanitizesStructuredProgressFromError(t *testing.T) {
	manager := &Manager{runner: NewRunner()}
	_, err := manager.runPreparedDownload(context.Background(), downloadHelperCommand("fail"), nil)

	var processErr *ProcessError
	if !errors.As(err, &processErr) {
		t.Fatalf("expected ProcessError, got %T: %v", err, err)
	}
	if strings.Contains(processErr.Error(), progressPrefix) {
		t.Fatalf("structured progress leaked into error: %v", processErr)
	}
	if !strings.Contains(processErr.Error(), "actual failure") {
		t.Fatalf("missing yt-dlp error detail: %v", processErr)
	}
}

func TestDownloadHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_DOWNLOAD_HELPER_PROCESS") != "1" {
		return
	}
}

func init() {
	if os.Getenv("GO_WANT_DOWNLOAD_HELPER_PROCESS") != "1" {
		return
	}

	mode := os.Args[len(os.Args)-1]
	switch mode {
	case "success":
		fmt.Fprintln(os.Stdout, progressPrefix+"downloading\t500\t1000\tNA\t250\t2\t50.0%")
		fmt.Fprintln(os.Stdout, progressPrefix+"finished\t1000\t1000\tNA\t0\t0\t100.0%")
		fmt.Fprintln(os.Stdout, finalPathPrefix+`C:\\Video\\done.mp4`)
		os.Exit(0)
	case "no-file":
		fmt.Fprintln(os.Stdout, progressPrefix+"finished\t1000\t1000\tNA\t0\t0\t100.0%")
		os.Exit(0)
	case "wait":
		time.Sleep(5 * time.Second)
		os.Exit(0)
	case "fail":
		fmt.Fprintln(os.Stderr, progressPrefix+"downloading\t500\t1000\tNA\t250\t2\t50.0%")
		fmt.Fprintln(os.Stderr, "ERROR: actual failure")
		os.Exit(7)
	default:
		os.Exit(2)
	}
}
