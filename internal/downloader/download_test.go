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
	case "unauthorized":
		fmt.Fprintln(os.Stderr, "ERROR: [chzzk:video] Failed to download MPD manifest: HTTP Error 401: Unauthorized")
		os.Exit(1)
	case "partial-conflict":
		fmt.Fprintln(os.Stderr, "ERROR: Initialization fragment found after media fragments, unable to download")
		os.Exit(1)
	default:
		os.Exit(2)
	}
}


func TestRunPreparedDownloadClassifiesAuthenticationRequired(t *testing.T) {
	manager := &Manager{runner: NewRunner()}
	_, err := manager.runPreparedDownload(
		context.Background(),
		downloadHelperCommand("unauthorized"),
		nil,
	)
	if err == nil {
		t.Fatal("expected authentication error")
	}

	kind, ok := downloadFailureKind(err)
	if !ok || kind != DownloadFailureAuthenticationRequired {
		t.Fatalf("unexpected failure kind: %q %v", kind, err)
	}
	if got := err.Error(); got != "로그인이 필요한 콘텐츠입니다. 연령 제한 또는 접근 권한이 필요한 영상일 수 있습니다." {
		t.Fatalf("unexpected user message: %q", got)
	}
	if strings.Contains(strings.ToLower(err.Error()), "yt-dlp") ||
		strings.Contains(strings.ToLower(err.Error()), "401") {
		t.Fatalf("internal error leaked to user: %v", err)
	}
}

func TestRunPreparedDownloadClassifiesPartialDataConflict(t *testing.T) {
	manager := &Manager{runner: NewRunner()}
	_, err := manager.runPreparedDownload(
		context.Background(),
		downloadHelperCommand("partial-conflict"),
		nil,
	)
	if err == nil {
		t.Fatal("expected partial data conflict")
	}

	kind, ok := downloadFailureKind(err)
	if !ok || kind != DownloadFailurePartialDataConflict {
		t.Fatalf("unexpected failure kind: %q %v", kind, err)
	}
	if got := err.Error(); got != "이전 다운로드의 임시 데이터와 충돌했습니다. 다운로드를 처음부터 다시 시도해 주세요." {
		t.Fatalf("unexpected user message: %q", got)
	}
}
