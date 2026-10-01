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
	if !ok || kind != DownloadFailureHLSInitializationFragmentOrder {
		t.Fatalf("unexpected failure kind: %q %v", kind, err)
	}
	if got := err.Error(); got != "영상 스트림 구조를 일반 방식으로 처리할 수 없어 대체 다운로드 방식으로 다시 시도합니다." {
		t.Fatalf("unexpected user message: %q", got)
	}
}


func useFastFallbackMonitor(t *testing.T) {
	t.Helper()
	previousPoll := ffmpegFallbackPollInterval
	previousStall := ffmpegFallbackStallTimeout
	ffmpegFallbackPollInterval = 5 * time.Millisecond
	ffmpegFallbackStallTimeout = 80 * time.Millisecond
	t.Cleanup(func() {
		ffmpegFallbackPollInterval = previousPoll
		ffmpegFallbackStallTimeout = previousStall
	})
}

func TestExecuteDownloadWithHLSFallbackRetriesWithFFmpegDownloader(t *testing.T) {
	useFastFallbackMonitor(t)

	outputDir := t.TempDir()
	request := DownloadRequest{
		URL:       "https://chzzk.naver.com/video/15461111",
		OutputDir: outputDir,
	}
	tempDir, err := temporaryDownloadDirForRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stalePath := tempDir + string(os.PathSeparator) + "stale.part"
	if err := os.WriteFile(stalePath, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	initialSpec := CommandSpec{Path: "yt-dlp.exe", Args: []string{"initial"}}
	attempts := 0
	var progressEvents []DownloadProgress
	result, usedSpec, err := executeDownloadWithHLSFallback(
		context.Background(),
		request,
		readyToolchain(t.TempDir()),
		initialSpec,
		func(progress DownloadProgress) {
			progressEvents = append(progressEvents, progress)
		},
		func(
			attemptCtx context.Context,
			spec CommandSpec,
			progressHandler ProgressHandler,
			lineHandler LineHandler,
		) (DownloadResult, error) {
			attempts++
			if attempts == 1 {
				return DownloadResult{
					DiagnosticLines: []OutputLine{{Stream: StreamStderr, Text: "native failure"}},
				}, &DownloadFailure{
					Kind:    DownloadFailureHLSInitializationFragmentOrder,
					Message: "fallback required",
					Cause:   errors.New("Initialization fragment found after media fragments"),
				}
			}
			if _, statErr := os.Stat(stalePath); !os.IsNotExist(statErr) {
				t.Fatalf("stale VOD temp data was not removed before fallback: %v", statErr)
			}
			found := false
			for index := 0; index+1 < len(spec.Args); index++ {
				if spec.Args[index] == "--downloader" && spec.Args[index+1] == "m3u8:ffmpeg" {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("ffmpeg HLS downloader was not selected: %#v", spec.Args)
			}

			if lineHandler != nil {
				lineHandler(OutputLine{Stream: StreamStderr, Text: "ffmpeg started"})
			}
			if err := os.WriteFile(
				tempDir+string(os.PathSeparator)+"fallback.part",
				make([]byte, 512),
				0o644,
			); err != nil {
				t.Fatal(err)
			}
			time.Sleep(20 * time.Millisecond)

			return DownloadResult{
				FinalPath:    "done.mp4",
				LastProgress: DownloadProgress{Status: "completed", Percent: 100},
			}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("unexpected attempt count: %d", attempts)
	}
	if result.FinalPath != "done.mp4" {
		t.Fatalf("unexpected fallback result: %#v", result)
	}
	if usedSpec.Path == initialSpec.Path && slices.Equal(usedSpec.Args, initialSpec.Args) {
		t.Fatalf("fallback command was not returned: %#v", usedSpec)
	}

	sawPreparing := false
	sawDownloading := false
	for _, progress := range progressEvents {
		sawPreparing = sawPreparing || progress.Status == progressStatusFallbackPreparing
		sawDownloading = sawDownloading ||
			(progress.Status == progressStatusFallbackDownloading && progress.DownloadedBytes >= 512)
	}
	if !sawPreparing || !sawDownloading {
		t.Fatalf("fallback activity was not surfaced: %#v", progressEvents)
	}
}

func TestExecuteDownloadWithHLSFallbackMergesDiagnosticsWhenFallbackFails(t *testing.T) {
	useFastFallbackMonitor(t)

	request := DownloadRequest{
		URL:       "https://chzzk.naver.com/video/15461111",
		OutputDir: t.TempDir(),
	}
	attempts := 0
	result, _, err := executeDownloadWithHLSFallback(
		context.Background(),
		request,
		readyToolchain(t.TempDir()),
		CommandSpec{Path: "yt-dlp.exe", Args: []string{"initial"}},
		nil,
		func(
			context.Context,
			CommandSpec,
			ProgressHandler,
			LineHandler,
		) (DownloadResult, error) {
			attempts++
			if attempts == 1 {
				return DownloadResult{
					DiagnosticLines: []OutputLine{{Stream: StreamStderr, Text: "first-attempt-output"}},
				}, &DownloadFailure{
					Kind:    DownloadFailureHLSInitializationFragmentOrder,
					Message: "fallback required",
					Cause:   errors.New("Initialization fragment found after media fragments"),
				}
			}
			return DownloadResult{
				DiagnosticLines: []OutputLine{{Stream: StreamStderr, Text: "fallback-output"}},
			}, errors.New("fallback failed")
		},
	)
	if err == nil {
		t.Fatal("expected fallback failure")
	}
	if kind, ok := downloadFailureKind(err); !ok || kind != DownloadFailureHLSInitializationFragmentOrder {
		t.Fatalf("unexpected fallback failure kind: %q %v", kind, err)
	}

	var text strings.Builder
	for _, line := range result.DiagnosticLines {
		text.WriteString(line.Text)
		text.WriteByte('\n')
	}
	diagnostics := text.String()
	for _, expected := range []string{
		"--- native HLS attempt ---",
		"first-attempt-output",
		"--- ffmpeg HLS fallback attempt ---",
		"fallback-output",
	} {
		if !strings.Contains(diagnostics, expected) {
			t.Fatalf("missing %q in diagnostics:\n%s", expected, diagnostics)
		}
	}
}

func TestExecuteDownloadWithHLSFallbackStopsStalledFallback(t *testing.T) {
	useFastFallbackMonitor(t)
	ffmpegFallbackStallTimeout = 30 * time.Millisecond

	request := DownloadRequest{
		URL:       "https://chzzk.naver.com/video/15461111",
		OutputDir: t.TempDir(),
	}
	attempts := 0
	result, _, err := executeDownloadWithHLSFallback(
		context.Background(),
		request,
		readyToolchain(t.TempDir()),
		CommandSpec{Path: "yt-dlp.exe", Args: []string{"initial"}},
		nil,
		func(
			attemptCtx context.Context,
			CommandSpec,
			ProgressHandler,
			LineHandler,
		) (DownloadResult, error) {
			attempts++
			if attempts == 1 {
				return DownloadResult{}, &DownloadFailure{
					Kind:    DownloadFailureHLSInitializationFragmentOrder,
					Message: "fallback required",
					Cause:   errors.New("Initialization fragment found after media fragments"),
				}
			}
			<-attemptCtx.Done()
			return DownloadResult{}, attemptCtx.Err()
		},
	)
	if err == nil || !strings.Contains(err.Error(), "진행되지 않아 중단") {
		t.Fatalf("unexpected stall error: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("unexpected attempt count: %d", attempts)
	}

	var diagnostics strings.Builder
	for _, line := range result.DiagnosticLines {
		diagnostics.WriteString(line.Text)
		diagnostics.WriteByte('\n')
	}
	if !strings.Contains(diagnostics.String(), "fallback monitor: no process output or temporary file growth") {
		t.Fatalf("stall diagnostic missing:\n%s", diagnostics.String())
	}
}

