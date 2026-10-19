package downloader

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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


func commandTemporaryDir(args []string) string {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == "--paths" && strings.HasPrefix(args[index+1], "temp:") {
			return strings.TrimPrefix(args[index+1], "temp:")
		}
	}
	return ""
}

func useFastFallbackMonitor(t *testing.T) {
	t.Helper()
	previousPoll := ffmpegFallbackPollInterval
	previousPreparation := ffmpegFallbackPreparationTimeout
	previousStall := ffmpegFallbackStallTimeout
	ffmpegFallbackPollInterval = 5 * time.Millisecond
	ffmpegFallbackPreparationTimeout = 80 * time.Millisecond
	ffmpegFallbackStallTimeout = 80 * time.Millisecond
	t.Cleanup(func() {
		ffmpegFallbackPollInterval = previousPoll
		ffmpegFallbackPreparationTimeout = previousPreparation
		ffmpegFallbackStallTimeout = previousStall
	})
}

func unavailableSplitRawRunner(
	context.Context,
	CommandSpec,
	LineHandler,
) error {
	return errors.New("split probe unavailable")
}

func TestExecuteDownloadWithHLSFallbackUsesDiscontinuitySplitBeforeFFmpeg(t *testing.T) {
	outputDir := t.TempDir()
	request := DownloadRequest{
		URL:       "https://chzzk.naver.com/video/15461111",
		OutputDir: outputDir,
	}
	toolchain := readyToolchain(t.TempDir())
	attempts := 0
	rawRuns := 0

	result, usedSpec, err := executeDownloadWithHLSFallback(
		context.Background(),
		request,
		toolchain,
		CommandSpec{Path: "yt-dlp.exe", Args: []string{"initial"}},
		nil,
		func(
			_ context.Context,
			spec CommandSpec,
			_ ProgressHandler,
			_ LineHandler,
		) (DownloadResult, error) {
			attempts++
			if attempts == 1 {
				return DownloadResult{
					DiagnosticLines: []OutputLine{{Stream: StreamStderr, Text: "native failure"}},
				}, &DownloadFailure{
					Kind:    DownloadFailureHLSInitializationFragmentOrder,
					Message: "split required",
					Cause:   errors.New("Initialization fragment found after media fragments"),
				}
			}

			if !slices.Contains(spec.Args, "--hls-split-discontinuity") {
				t.Fatalf("discontinuity split was not selected before ffmpeg: %#v", spec.Args)
			}
			formatID := argumentValue(spec.Args, "--format")
			if formatID != "1080p-0" && formatID != "1080p-1" {
				t.Fatalf("unexpected split format: %q", formatID)
			}
			splitDir := commandTemporaryDir(spec.Args)
			if splitDir == "" {
				t.Fatalf("split temp directory missing: %#v", spec.Args)
			}
			path := filepath.Join(splitDir, "sample [15461111]."+formatID+".mp4")
			if err := os.WriteFile(path, []byte(formatID), 0o644); err != nil {
				t.Fatal(err)
			}
			return DownloadResult{
				FinalPath: path,
				LastProgress: DownloadProgress{
					Status:          "completed",
					DownloadedBytes: int64(len(formatID)),
					Percent:         100,
				},
			}, nil
		},
		func(_ context.Context, spec CommandSpec, lineHandler LineHandler) error {
			rawRuns++
			if spec.Path == toolchain.YTDLP.Path {
				if slices.Contains(spec.Args, "--hls-split-discontinuity") {
					if lineHandler != nil {
						lineHandler(OutputLine{
							Stream: StreamStdout,
							Text: hlsSplitFormatProbePrefix +
								`[{"format_id":"1080p-0","vcodec":"avc1","acodec":"mp4a","ext":"mp4"},{"format_id":"1080p-1","vcodec":"avc1","acodec":"mp4a","ext":"mp4"}]`,
						})
					}
					return nil
				}
				if lineHandler != nil {
					lineHandler(OutputLine{
						Stream: StreamStdout,
						Text: hlsFormatProbeRequestedPrefix +
							`[{"format_id":"1080p","vcodec":"avc1","acodec":"mp4a","ext":"mp4"}]`,
					})
				}
				return nil
			}
			if spec.Path == toolchain.FFmpeg.Path {
				if len(spec.Args) == 0 {
					t.Fatal("ffmpeg concat args missing")
				}
				return os.WriteFile(spec.Args[len(spec.Args)-1], []byte("joined"), 0o644)
			}
			return fmt.Errorf("unexpected raw command: %s", spec.Path)
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 3 {
		t.Fatalf("ffmpeg yt-dlp fallback should not run after split success: attempts=%d", attempts)
	}
	if rawRuns != 3 {
		t.Fatalf("expected selected-format probe + split-format probe + concat, got %d", rawRuns)
	}
	if !hasArgumentPair(usedSpec.Args, "--format", "1080p-1") {
		t.Fatalf("unexpected used split spec: %#v", usedSpec)
	}
	expected := filepath.Join(outputDir, "sample [15461111].mp4")
	if result.FinalPath != expected {
		t.Fatalf("unexpected final path: %q", result.FinalPath)
	}
	data, readErr := os.ReadFile(expected)
	if readErr != nil || string(data) != "joined" {
		t.Fatalf("assembled split result mismatch: %q %v", string(data), readErr)
	}
}

func TestExecuteDownloadWithHLSFallbackRetriesWithFFmpegDownloader(t *testing.T) {
	useFastFallbackMonitor(t)

	outputDir := t.TempDir()
	request := DownloadRequest{
		URL:       "https://chzzk.naver.com/video/15461111",
		OutputDir: outputDir,
	}
	nativeTempDir, err := nativeTemporaryDownloadDirForRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(nativeTempDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stalePath := filepath.Join(nativeTempDir, "stale.part")
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
			if _, statErr := os.Stat(stalePath); statErr != nil {
				t.Fatalf("native temp data must not be deleted before fallback: %v", statErr)
			}
			fallbackTempDir := commandTemporaryDir(spec.Args)
			if fallbackTempDir == "" {
				t.Fatalf("fallback temp directory missing: %#v", spec.Args)
			}
			if filepath.Clean(fallbackTempDir) == filepath.Clean(nativeTempDir) {
				t.Fatalf("fallback reused native temp directory: %q", fallbackTempDir)
			}
			if !strings.HasPrefix(filepath.Base(fallbackTempDir), fallbackTemporaryDirectoryNamePrefix) {
				t.Fatalf("unexpected fallback temp directory: %q", fallbackTempDir)
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
				lineHandler(OutputLine{
					Stream: StreamStderr,
					Text:   `[debug] Invoking ffmpeg downloader on "https://example.com/master.m3u8?token=secret"`,
				})
			}
			if err := os.WriteFile(
				filepath.Join(fallbackTempDir, "fallback.part"),
				make([]byte, 512),
				0o644,
			); err != nil {
				t.Fatal(err)
			}
			time.Sleep(20 * time.Millisecond)

			finalPath := filepath.Join(fallbackTempDir, "done.mp4")
			if err := os.WriteFile(finalPath, []byte("completed"), 0o644); err != nil {
				t.Fatal(err)
			}
			return DownloadResult{
				FinalPath:    finalPath,
				LastProgress: DownloadProgress{Status: "completed", Percent: 100},
			}, nil
		},
		unavailableSplitRawRunner,
	)
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("unexpected attempt count: %d", attempts)
	}
	expectedFinalPath := filepath.Join(outputDir, "done.mp4")
	if result.FinalPath != expectedFinalPath {
		t.Fatalf("unexpected fallback result: %#v", result)
	}
	if data, statErr := os.ReadFile(expectedFinalPath); statErr != nil || string(data) != "completed" {
		t.Fatalf("fallback result was not promoted: %q %v", string(data), statErr)
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
		unavailableSplitRawRunner,
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
		"--- discontinuity split HLS attempt ---",
		"discontinuity split error:",
		"--- ffmpeg HLS fallback attempt ---",
		"fallback-output",
	} {
		if !strings.Contains(diagnostics, expected) {
			t.Fatalf("missing %q in diagnostics:\n%s", expected, diagnostics)
		}
	}
}

func TestRunMonitoredHLSFallbackCapturesImmediateFileGrowth(t *testing.T) {
	useFastFallbackMonitor(t)

	tempDir := t.TempDir()
	var events []DownloadProgress
	result, err, reason := runMonitoredHLSFallback(
		context.Background(),
		tempDir,
		CommandSpec{Path: "yt-dlp.exe"},
		func(progress DownloadProgress) {
			events = append(events, progress)
		},
		func(
			context.Context,
			CommandSpec,
			ProgressHandler,
			LineHandler,
		) (DownloadResult, error) {
			if err := os.WriteFile(filepath.Join(tempDir, "fast.part"), make([]byte, 4096), 0o644); err != nil {
				t.Fatal(err)
			}
			time.Sleep(20 * time.Millisecond)
			return DownloadResult{}, errors.New("expected test stop")
		},
	)
	if err == nil || reason != fallbackMonitorStopNone {
		t.Fatalf("unexpected monitored result: %#v %v %q", result, err, reason)
	}
	found := false
	for _, event := range events {
		if event.Status == progressStatusFallbackDownloading && event.DownloadedBytes >= 4096 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("immediate fallback growth was not observed: %#v", events)
	}
}

func TestExecuteDownloadWithHLSFallbackStopsPreparationTimeout(t *testing.T) {
	useFastFallbackMonitor(t)
	ffmpegFallbackPreparationTimeout = 30 * time.Millisecond
	ffmpegFallbackStallTimeout = 200 * time.Millisecond

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
			lineHandler LineHandler,
		) (DownloadResult, error) {
			attempts++
			if attempts == 1 {
				return DownloadResult{}, &DownloadFailure{
					Kind:    DownloadFailureHLSInitializationFragmentOrder,
					Message: "fallback required",
					Cause:   errors.New("Initialization fragment found after media fragments"),
				}
			}

			ticker := time.NewTicker(5 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-attemptCtx.Done():
					return DownloadResult{}, attemptCtx.Err()
				case <-ticker.C:
					if lineHandler != nil {
						lineHandler(OutputLine{
							Stream: StreamStderr,
							Text:   "[debug] Extracting URL metadata",
						})
					}
				}
			}
		},
		unavailableSplitRawRunner,
	)
	if err == nil || !strings.Contains(err.Error(), "대체 다운로드 준비가") {
		t.Fatalf("unexpected preparation timeout error: %v", err)
	}
	if result.LastProgress.Status != progressStatusFallbackPreparing {
		t.Fatalf("unexpected preparation progress: %#v", result.LastProgress)
	}

	var diagnostics strings.Builder
	for _, line := range result.DiagnosticLines {
		diagnostics.WriteString(line.Text)
		diagnostics.WriteByte('\n')
	}
	if !strings.Contains(diagnostics.String(), "ffmpeg downloader did not start") {
		t.Fatalf("preparation timeout diagnostic missing:\n%s", diagnostics.String())
	}
}

func TestExecuteDownloadWithHLSFallbackStopsStalledFallback(t *testing.T) {
	useFastFallbackMonitor(t)
	ffmpegFallbackPreparationTimeout = 200 * time.Millisecond
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
			lineHandler LineHandler,
		) (DownloadResult, error) {
			attempts++
			if attempts == 1 {
				return DownloadResult{}, &DownloadFailure{
					Kind:    DownloadFailureHLSInitializationFragmentOrder,
					Message: "fallback required",
					Cause:   errors.New("Initialization fragment found after media fragments"),
				}
			}
			if lineHandler != nil {
				lineHandler(OutputLine{
					Stream: StreamStderr,
					Text:   `[debug] Invoking ffmpeg downloader on "https://example.com/master.m3u8?token=secret"`,
				})
			}

			ticker := time.NewTicker(5 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-attemptCtx.Done():
					return DownloadResult{}, attemptCtx.Err()
				case <-ticker.C:
					if lineHandler != nil {
						lineHandler(OutputLine{Stream: StreamStderr, Text: "ffmpeg heartbeat"})
					}
				}
			}
		},
		unavailableSplitRawRunner,
	)
	if err == nil || !strings.Contains(err.Error(), "구간 분할 처리 후에도 대체 다운로드가") {
		t.Fatalf("unexpected stall error: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("unexpected attempt count: %d", attempts)
	}
	if result.LastProgress.Status != progressStatusFallbackDownloading {
		t.Fatalf("unexpected stalled progress: %#v", result.LastProgress)
	}

	var diagnostics strings.Builder
	for _, line := range result.DiagnosticLines {
		diagnostics.WriteString(line.Text)
		diagnostics.WriteByte('\n')
	}
	if !strings.Contains(diagnostics.String(), "no fallback file growth") {
		t.Fatalf("stall diagnostic missing:\n%s", diagnostics.String())
	}
}

func TestIsFFmpegDownloaderStartLine(t *testing.T) {
	for _, line := range []string{
		`[debug] Invoking ffmpeg downloader on "https://example.com/master.m3u8"`,
		`[debug] ffmpeg command line: ffmpeg -i https://example.com/master.m3u8`,
	} {
		if !isFFmpegDownloaderStartLine(line) {
			t.Fatalf("expected downloader start line: %q", line)
		}
	}
	if isFFmpegDownloaderStartLine("[debug] Extracting URL metadata") {
		t.Fatal("metadata output must not mark ffmpeg downloader as started")
	}
}

func TestAudioHLSFallbackUsesSplitAndCreatesMP3(t *testing.T) {
	outputDir := t.TempDir()
	toolchain := readyToolchain(t.TempDir())
	request := DownloadRequest{
		DownloadType: DownloadTypeAudio,
		URL: "https://chzzk.naver.com/video/15461111",
		OutputDir: outputDir,
		FormatSelector: "bestaudio/best",
	}
	var attempts int
	var conversions int

	result, _, err := executeDownloadWithHLSFallback(
		context.Background(), request, toolchain,
		CommandSpec{Path: toolchain.YTDLP.Path, Args: []string{"initial"}},
		nil,
		func(_ context.Context, spec CommandSpec, _ ProgressHandler, _ LineHandler) (DownloadResult, error) {
			attempts++
			if attempts == 1 {
				return DownloadResult{}, &DownloadFailure{
					Kind: DownloadFailureHLSInitializationFragmentOrder,
					Message: "HLS recovery required",
					Cause: errors.New("Initialization fragment found after media fragments"),
				}
			}
			if !slices.Contains(spec.Args, "--hls-split-discontinuity") {
				t.Fatalf("split HLS was skipped: %#v", spec.Args)
			}
			formatID := argumentValue(spec.Args, "--format")
			if formatID != "audio-0" && formatID != "audio-1" {
				t.Fatalf("unexpected audio format: %s", formatID)
			}
			tempDir := commandTemporaryDir(spec.Args)
			path := filepath.Join(tempDir, "sample [15461111]."+formatID+".m4a")
			if err := os.WriteFile(path, []byte("audio-segment"), 0o644); err != nil {
				t.Fatal(err)
			}
			return DownloadResult{
				FinalPath: path,
				LastProgress: DownloadProgress{Status: "completed", Percent: 100},
			}, nil
		},
		func(_ context.Context, spec CommandSpec, emit LineHandler) error {
			if spec.Path == toolchain.YTDLP.Path {
				if slices.Contains(spec.Args, "--hls-split-discontinuity") {
					emit(OutputLine{
						Stream: StreamStdout,
						Text: hlsSplitFormatProbePrefix +
							`[{"format_id":"audio-0","vcodec":"none","acodec":"mp4a","ext":"m4a"},{"format_id":"audio-1","vcodec":"none","acodec":"mp4a","ext":"m4a"}]`,
					})
					return nil
				}
				if !hasArgumentPair(spec.Args, "--format", "bestaudio/best") {
					t.Fatalf("audio probe did not use bestaudio: %#v", spec.Args)
				}
				emit(OutputLine{
					Stream: StreamStdout,
					Text: hlsFormatProbeSinglePrefix +
						`{"format_id":"audio","vcodec":"none","acodec":"mp4a","ext":"m4a"}`,
				})
				return nil
			}
			if spec.Path != toolchain.FFmpeg.Path {
				t.Fatalf("unexpected process: %s", spec.Path)
			}
			output := spec.Args[len(spec.Args)-1]
			if slices.Contains(spec.Args, "-codec:a") {
				conversions++
				if !hasArgumentPair(spec.Args, "-map", "0:a:0") ||
					!hasArgumentPair(spec.Args, "-codec:a", "libmp3lame") {
					t.Fatalf("MP3 codec selection missing: %#v", spec.Args)
				}
				return os.WriteFile(output, []byte("mp3-result"), 0o644)
			}
			if !hasArgumentPair(spec.Args, "-f", "concat") {
				t.Fatalf("missing HLS audio concat: %#v", spec.Args)
			}
			return os.WriteFile(output, []byte("joined"), 0o644)
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 3 || conversions != 1 {
		t.Fatalf("expected initial + 2 split downloads and 1 mp3 conversion: attempts=%d converts=%d", attempts, conversions)
	}
	want := filepath.Join(outputDir, "sample [15461111].mp3")
	if result.FinalPath != want || result.LastProgress.Status != "completed" {
		t.Fatalf("unexpected final result: %#v", result)
	}
	if data, err := os.ReadFile(want); err != nil || string(data) != "mp3-result" {
		t.Fatalf("MP3 output was not promoted: %q %v", string(data), err)
	}
}

func TestAudioHLSFallbackUsesFFmpegAfterSplitFails(t *testing.T) {
	useFastFallbackMonitor(t)
	outputDir := t.TempDir()
	request := DownloadRequest{
		DownloadType: DownloadTypeAudio,
		URL: "https://chzzk.naver.com/video/15461111",
		OutputDir: outputDir,
	}
	toolchain := readyToolchain(t.TempDir())
	attempts := 0
	var events []DownloadProgress
	result, spec, err := executeDownloadWithHLSFallback(
		context.Background(), request, toolchain,
		CommandSpec{Path: "yt-dlp.exe", Args: []string{"initial"}},
		func(progress DownloadProgress) { events = append(events, progress) },
		func(_ context.Context, spec CommandSpec, _ ProgressHandler, report LineHandler) (DownloadResult, error) {
			attempts++
			if attempts == 1 {
				return DownloadResult{
					DiagnosticLines: []OutputLine{{Stream: StreamStderr, Text: "audio-native-failed"}},
				}, &DownloadFailure{
					Kind: DownloadFailureHLSInitializationFragmentOrder,
					Message: "HLS recovery required",
					Cause: errors.New("Initialization fragment found after media fragments"),
				}
			}
			for _, pair := range [][2]string{
				{"--downloader", "m3u8:ffmpeg"},
				{"--format", "bestaudio/best"},
				{"--audio-format", "mp3"},
			} {
				if !hasArgumentPair(spec.Args, pair[0], pair[1]) {
					t.Fatalf("audio fallback option missing %v: %#v", pair, spec.Args)
				}
			}
			if !slices.Contains(spec.Args, "--extract-audio") {
				t.Fatal("MP3 extraction was lost in ffmpeg fallback")
			}
			if report != nil {
				report(OutputLine{Stream: StreamStderr, Text: "[debug] Invoking ffmpeg downloader"})
			}
			tempDir := commandTemporaryDir(spec.Args)
			path := filepath.Join(tempDir, "done.mp3")
			if err := os.WriteFile(path, []byte("fallback-mp3"), 0o644); err != nil {
				t.Fatal(err)
			}
			return DownloadResult{
				FinalPath: path,
				LastProgress: DownloadProgress{Status: "completed", Percent: 100},
			}, nil
		},
		unavailableSplitRawRunner,
	)
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 2 || !hasArgumentPair(spec.Args, "--downloader", "m3u8:ffmpeg") {
		t.Fatalf("wrong fallback sequence, attempts=%d spec=%#v", attempts, spec)
	}
	want := filepath.Join(outputDir, "done.mp3")
	if result.FinalPath != want {
		t.Fatalf("unexpected MP3 final path: %q", result.FinalPath)
	}
	if data, err := os.ReadFile(want); err != nil || string(data) != "fallback-mp3" {
		t.Fatalf("ffmpeg fallback MP3 was not promoted: %q %v", string(data), err)
	}
	seenPreparing := false
	for _, event := range events {
		if event.Status == progressStatusFallbackPreparing {
			seenPreparing = true
		}
	}
	if !seenPreparing {
		t.Fatalf("fallback preparing status missing: %#v", events)
	}
}

func TestAudioHLSFallbackDoesNotRetryAuthenticationFailure(t *testing.T) {
	attempts := 0
	denied := NewAuthenticationRequiredFailure(errors.New("401 unauthorized"))
	_, _, err := executeDownloadWithHLSFallback(
		context.Background(),
		DownloadRequest{DownloadType: DownloadTypeAudio, URL: "https://chzzk.naver.com/video/100", OutputDir: t.TempDir()},
		readyToolchain(t.TempDir()),
		CommandSpec{Path: "yt-dlp.exe"},
		nil,
		func(context.Context, CommandSpec, ProgressHandler, LineHandler) (DownloadResult, error) {
			attempts++
			return DownloadResult{}, denied
		},
		func(context.Context, CommandSpec, LineHandler) error {
			t.Fatal("authentication errors must not start a split probe")
			return nil
		},
	)
	if !errors.Is(err, denied) || attempts != 1 {
		t.Fatalf("unexpected authentication retry: attempts=%d err=%v", attempts, err)
	}
}

func TestAudioHLSFallbackRetriesFFmpegWhenSplitMP3ConversionFails(t *testing.T) {
	useFastFallbackMonitor(t)
	outputDir := t.TempDir()
	toolchain := readyToolchain(t.TempDir())
	request := DownloadRequest{
		DownloadType: DownloadTypeAudio,
		URL: "https://chzzk.naver.com/video/15461111",
		OutputDir: outputDir,
	}
	attempts := 0
	result, spec, err := executeDownloadWithHLSFallback(
		context.Background(), request, toolchain,
		CommandSpec{Path: "yt-dlp.exe", Args: []string{"initial"}},
		nil,
		func(_ context.Context, spec CommandSpec, _ ProgressHandler, _ LineHandler) (DownloadResult, error) {
			attempts++
			switch attempts {
			case 1:
				return DownloadResult{}, &DownloadFailure{
					Kind: DownloadFailureHLSInitializationFragmentOrder,
					Message: "HLS recovery required",
					Cause: errors.New("Initialization fragment found after media fragments"),
				}
			case 2:
				if !slices.Contains(spec.Args, "--hls-split-discontinuity") {
					t.Fatalf("split was not attempted first: %#v", spec.Args)
				}
				path := filepath.Join(commandTemporaryDir(spec.Args), "sample [15461111].audio-0.m4a")
				if err := os.WriteFile(path, []byte("original audio"), 0o644); err != nil {
					t.Fatal(err)
				}
				return DownloadResult{FinalPath: path}, nil
			case 3:
				if !hasArgumentPair(spec.Args, "--downloader", "m3u8:ffmpeg") ||
					!hasArgumentPair(spec.Args, "--audio-format", "mp3") {
					t.Fatalf("final ffmpeg MP3 attempt missing: %#v", spec.Args)
				}
				path := filepath.Join(commandTemporaryDir(spec.Args), "recovered.mp3")
				if err := os.WriteFile(path, []byte("recovered"), 0o644); err != nil {
					t.Fatal(err)
				}
				return DownloadResult{
					FinalPath: path,
					LastProgress: DownloadProgress{Status: "completed", Percent: 100},
				}, nil
			default:
				t.Fatalf("unexpected extra fallback attempt: %d", attempts)
				return DownloadResult{}, nil
			}
		},
		func(_ context.Context, spec CommandSpec, lineHandler LineHandler) error {
			if spec.Path == toolchain.YTDLP.Path {
				if slices.Contains(spec.Args, "--hls-split-discontinuity") {
					lineHandler(OutputLine{
						Stream: StreamStdout,
						Text: hlsSplitFormatProbePrefix +
							`[{"format_id":"audio-0","vcodec":"none","acodec":"mp4a","ext":"m4a"}]`,
					})
				} else {
					lineHandler(OutputLine{
						Stream: StreamStdout,
						Text: hlsFormatProbeSinglePrefix +
							`{"format_id":"audio","vcodec":"none","acodec":"mp4a","ext":"m4a"}`,
					})
				}
				return nil
			}
			if spec.Path != toolchain.FFmpeg.Path || !hasArgumentPair(spec.Args, "-codec:a", "libmp3lame") {
				t.Fatalf("unexpected conversion command: %#v", spec)
			}
			lineHandler(OutputLine{Stream: StreamStderr, Text: "MP3 conversion failure diagnostic"})
			return errors.New("MP3 conversion failed")
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 3 || !hasArgumentPair(spec.Args, "--downloader", "m3u8:ffmpeg") {
		t.Fatalf("expected conversion error to trigger final ffmpeg attempt: attempts=%d spec=%#v", attempts, spec)
	}
	want := filepath.Join(outputDir, "recovered.mp3")
	if result.FinalPath != want {
		t.Fatalf("unexpected recovered MP3 path: %s", result.FinalPath)
	}
	if data, err := os.ReadFile(want); err != nil || string(data) != "recovered" {
		t.Fatalf("recovered MP3 missing: %q %v", string(data), err)
	}
	joined := ""
	for _, line := range result.DiagnosticLines {
		joined += line.Text + "\n"
	}
	if !strings.Contains(joined, "MP3 conversion failure diagnostic") {
		t.Fatalf("conversion diagnostics were lost: %s", joined)
	}
}
