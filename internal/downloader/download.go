package downloader

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type ProgressHandler func(DownloadProgress)

type DownloadResult struct {
	FinalPath       string           `json:"finalPath"`
	LogPath         string           `json:"logPath,omitempty"`
	LastProgress    DownloadProgress `json:"lastProgress"`
	DiagnosticLines []OutputLine     `json:"-"`
}

var (
	ffmpegFallbackPollInterval       = time.Second
	ffmpegFallbackPreparationTimeout = 120 * time.Second
	ffmpegFallbackStallTimeout       = 60 * time.Second
)

type fallbackMonitorStopReason string

const (
	fallbackMonitorStopNone               fallbackMonitorStopReason = ""
	fallbackMonitorStopPreparationTimeout fallbackMonitorStopReason = "preparation_timeout"
	fallbackMonitorStopDownloadStalled    fallbackMonitorStopReason = "download_stalled"
)

type downloadAttemptRunner func(
	context.Context,
	CommandSpec,
	ProgressHandler,
	LineHandler,
) (DownloadResult, error)

func (m *Manager) Download(ctx context.Context, request DownloadRequest, handler ProgressHandler) (DownloadResult, error) {
	spec, toolchain, err := m.Prepare(ctx, request)
	if err != nil {
		result := DownloadResult{}
		if logPath, logErr := writeDownloadFailureLog(request, spec, result, err); logErr == nil {
			result.LogPath = logPath
		}
		return result, err
	}
	result, usedSpec, err := executeDownloadWithHLSFallback(
		ctx,
		request,
		toolchain,
		spec,
		handler,
		func(
			attemptCtx context.Context,
			attempt CommandSpec,
			progressHandler ProgressHandler,
			lineHandler LineHandler,
		) (DownloadResult, error) {
			return m.runPreparedDownloadObserved(
				attemptCtx,
				attempt,
				progressHandler,
				lineHandler,
			)
		},
		m.runner.Run,
	)
	if err != nil {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			if logPath, logErr := writeDownloadFailureLog(request, usedSpec, result, err); logErr == nil {
				result.LogPath = logPath
			}
		}
		return result, err
	}
	_ = cleanupTemporaryDownloadRequest(request)
	return result, nil
}

func executeDownloadWithHLSFallback(
	ctx context.Context,
	request DownloadRequest,
	toolchain ToolchainStatus,
	initialSpec CommandSpec,
	handler ProgressHandler,
	run downloadAttemptRunner,
	rawRun rawCommandRunner,
) (DownloadResult, CommandSpec, error) {
	firstResult, firstErr := run(ctx, initialSpec, handler, nil)
	if firstErr == nil || !isDownloadFailureKind(firstErr, DownloadFailureHLSInitializationFragmentOrder) {
		return firstResult, initialSpec, firstErr
	}
	if err := ctx.Err(); err != nil {
		return firstResult, initialSpec, fmt.Errorf("다운로드가 취소되었습니다: %w", err)
	}

	splitResult, splitSpec, splitErr := executeDiscontinuitySplitFallback(
		ctx,
		request,
		toolchain,
		handler,
		run,
		rawRun,
	)
	if splitErr == nil {
		return splitResult, splitSpec, nil
	}
	if errors.Is(splitErr, context.Canceled) || errors.Is(splitErr, context.DeadlineExceeded) {
		return splitResult, splitSpec, splitErr
	}
	if err := ctx.Err(); err != nil {
		return splitResult, splitSpec, fmt.Errorf("다운로드가 취소되었습니다: %w", err)
	}

	fallbackRunID := strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
	tempDir, err := fallbackTemporaryDownloadDirForRequest(request, fallbackRunID)
	if err != nil {
		return splitResult, splitSpec, &DownloadFailure{
			Kind:    DownloadFailureHLSInitializationFragmentOrder,
			Message: "영상 스트림 구조 문제를 감지했지만 대체 다운로드를 준비하지 못했습니다.",
			Cause:   err,
		}
	}
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		return splitResult, splitSpec, &DownloadFailure{
			Kind:    DownloadFailureHLSInitializationFragmentOrder,
			Message: "영상 스트림 구조 문제를 감지했지만 대체 다운로드를 준비하지 못했습니다.",
			Cause:   fmt.Errorf("다운로드 임시 폴더를 만들 수 없습니다: %w", err),
		}
	}

	fallbackSpec, err := buildDownloadCommandWithTempDir(toolchain, request, true, tempDir)
	if err != nil {
		return splitResult, splitSpec, &DownloadFailure{
			Kind:    DownloadFailureHLSInitializationFragmentOrder,
			Message: "영상 스트림 구조 문제를 감지했지만 대체 다운로드를 준비하지 못했습니다.",
			Cause:   err,
		}
	}

	fallbackResult, fallbackErr, stopReason := runMonitoredHLSFallback(
		ctx,
		tempDir,
		fallbackSpec,
		handler,
		run,
	)
	switch stopReason {
	case fallbackMonitorStopPreparationTimeout:
		seconds := int(ffmpegFallbackPreparationTimeout.Seconds())
		fallbackResult.LastProgress.Status = progressStatusFallbackPreparing
		fallbackResult.DiagnosticLines = appendDiagnosticLine(
			fallbackResult.DiagnosticLines,
			OutputLine{
				Stream: StreamStderr,
				Text: fmt.Sprintf(
					"fallback monitor: ffmpeg downloader did not start within %d seconds",
					seconds,
				),
			},
		)
	case fallbackMonitorStopDownloadStalled:
		seconds := int(ffmpegFallbackStallTimeout.Seconds())
		fallbackResult.LastProgress.Status = progressStatusFallbackDownloading
		fallbackResult.DiagnosticLines = appendDiagnosticLine(
			fallbackResult.DiagnosticLines,
			OutputLine{
				Stream: StreamStderr,
				Text: fmt.Sprintf(
					"fallback monitor: no fallback file growth for %d seconds after downloader start",
					seconds,
				),
			},
		)
	}
	fallbackResult.DiagnosticLines = mergeHLSFallbackDiagnostics(
		firstResult.DiagnosticLines,
		firstErr,
		splitResult.DiagnosticLines,
		splitErr,
		fallbackResult.DiagnosticLines,
	)

	switch stopReason {
	case fallbackMonitorStopPreparationTimeout:
		seconds := int(ffmpegFallbackPreparationTimeout.Seconds())
		return fallbackResult, fallbackSpec, &DownloadFailure{
			Kind:    DownloadFailureHLSInitializationFragmentOrder,
			Message: fmt.Sprintf(
				"대체 다운로드 준비가 %d초 동안 완료되지 않아 중단했습니다.",
				seconds,
			),
			Cause: fmt.Errorf(
				"ffmpeg HLS fallback downloader did not start within %d seconds",
				seconds,
			),
		}
	case fallbackMonitorStopDownloadStalled:
		seconds := int(ffmpegFallbackStallTimeout.Seconds())
		return fallbackResult, fallbackSpec, &DownloadFailure{
			Kind:    DownloadFailureHLSInitializationFragmentOrder,
			Message: fmt.Sprintf(
				"구간 분할 처리 후에도 대체 다운로드가 %d초 동안 진행되지 않아 중단했습니다.",
				seconds,
			),
			Cause: fmt.Errorf(
				"ffmpeg HLS fallback stalled without file growth for %d seconds after downloader start",
				seconds,
			),
		}
	}
	if fallbackErr != nil {
		if errors.Is(fallbackErr, context.Canceled) || errors.Is(fallbackErr, context.DeadlineExceeded) {
			return fallbackResult, fallbackSpec, fallbackErr
		}
		return fallbackResult, fallbackSpec, &DownloadFailure{
			Kind:    DownloadFailureHLSInitializationFragmentOrder,
			Message: "HLS 구간 분할과 대체 다운로드 방식까지 시도했지만 실패했습니다.",
			Cause:   fallbackErr,
		}
	}

	promotedPath, err := promoteFallbackDownload(tempDir, request.OutputDir, fallbackResult.FinalPath)
	if err != nil {
		return fallbackResult, fallbackSpec, &DownloadFailure{
			Kind:    DownloadFailureHLSInitializationFragmentOrder,
			Message: "대체 방식 다운로드는 완료했지만 최종 파일을 다운로드 폴더로 이동하지 못했습니다.",
			Cause:   err,
		}
	}
	fallbackResult.FinalPath = promotedPath
	return fallbackResult, fallbackSpec, nil
}

func runMonitoredHLSFallback(
	ctx context.Context,
	tempDir string,
	spec CommandSpec,
	handler ProgressHandler,
	run downloadAttemptRunner,
) (DownloadResult, error, fallbackMonitorStopReason) {
	fallbackCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	type outcome struct {
		result DownloadResult
		err    error
	}

	outcomes := make(chan outcome, 1)
	downloaderStartedSignal := make(chan struct{}, 1)
	markDownloaderStarted := func() {
		select {
		case downloaderStartedSignal <- struct{}{}:
		default:
		}
	}

	if handler != nil {
		handler(DownloadProgress{Status: progressStatusFallbackPreparing})
	}

	startedAt := time.Now()
	lastSample := startedAt
	lastGrowth := startedAt
	lastBytes, _ := temporaryDownloadSize(tempDir)

	go func() {
		result, err := run(
			fallbackCtx,
			spec,
			func(progress DownloadProgress) {
				if progress.Status != "completed" {
					markDownloaderStarted()
					progress.Status = progressStatusFallbackDownloading
				}
				if handler != nil {
					handler(progress)
				}
			},
			func(line OutputLine) {
				if isFFmpegDownloaderStartLine(line.Text) {
					markDownloaderStarted()
				}
			},
		)
		outcomes <- outcome{result: result, err: err}
	}()

	pollInterval := ffmpegFallbackPollInterval
	if pollInterval <= 0 {
		pollInterval = time.Second
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	downloaderStarted := false

	reportDownloading := func() {
		if downloaderStarted {
			return
		}
		downloaderStarted = true
		lastGrowth = time.Now()
		if handler != nil {
			handler(DownloadProgress{
				Status:          progressStatusFallbackDownloading,
				DownloadedBytes: lastBytes,
			})
		}
	}

	for {
		select {
		case current := <-outcomes:
			return current.result, current.err, fallbackMonitorStopNone

		case <-ctx.Done():
			cancel()
			current := <-outcomes
			return current.result, current.err, fallbackMonitorStopNone

		case <-downloaderStartedSignal:
			reportDownloading()

		case now := <-ticker.C:
			currentBytes, sizeErr := temporaryDownloadSize(tempDir)
			if sizeErr == nil && currentBytes != lastBytes {
				reportDownloading()

				elapsed := now.Sub(lastSample).Seconds()
				speed := 0.0
				if currentBytes > lastBytes && elapsed > 0 {
					speed = float64(currentBytes-lastBytes) / elapsed
				}

				lastGrowth = now
				lastBytes = currentBytes
				if handler != nil {
					handler(DownloadProgress{
						Status:              progressStatusFallbackDownloading,
						DownloadedBytes:     currentBytes,
						SpeedBytesPerSecond: speed,
					})
				}
			}
			lastSample = now

			if !downloaderStarted &&
				ffmpegFallbackPreparationTimeout > 0 &&
				now.Sub(startedAt) >= ffmpegFallbackPreparationTimeout {
				cancel()
				current := <-outcomes
				return current.result, current.err, fallbackMonitorStopPreparationTimeout
			}

			if downloaderStarted &&
				ffmpegFallbackStallTimeout > 0 &&
				now.Sub(lastGrowth) >= ffmpegFallbackStallTimeout {
				cancel()
				current := <-outcomes
				return current.result, current.err, fallbackMonitorStopDownloadStalled
			}
		}
	}
}

func isFFmpegDownloaderStartLine(line string) bool {
	lower := strings.ToLower(line)
	return strings.Contains(lower, "invoking ffmpeg downloader") ||
		strings.Contains(lower, "ffmpeg command line")
}

func mergeHLSFallbackDiagnostics(
	first []OutputLine,
	firstErr error,
	split []OutputLine,
	splitErr error,
	ffmpeg []OutputLine,
) []OutputLine {
	const (
		nativeLimit = 70
		splitLimit  = 110
	)

	merged := make([]OutputLine, 0, maxDiagnosticLines)
	merged = appendDiagnosticLine(merged, OutputLine{
		Stream: StreamStderr,
		Text:   "--- native HLS attempt ---",
	})
	if len(first) > nativeLimit {
		first = first[len(first)-nativeLimit:]
	}
	merged = appendDiagnosticLines(merged, first...)

	rawFirstErr := firstErr
	if unwrapped := errors.Unwrap(firstErr); unwrapped != nil {
		rawFirstErr = unwrapped
	}
	merged = appendDiagnosticLine(merged, OutputLine{
		Stream: StreamStderr,
		Text:   fmt.Sprintf("native HLS error: %v", rawFirstErr),
	})

	merged = appendDiagnosticLine(merged, OutputLine{
		Stream: StreamStderr,
		Text:   "--- discontinuity split HLS attempt ---",
	})
	if len(split) > splitLimit {
		split = split[len(split)-splitLimit:]
	}
	merged = appendDiagnosticLines(merged, split...)
	merged = appendDiagnosticLine(merged, OutputLine{
		Stream: StreamStderr,
		Text:   fmt.Sprintf("discontinuity split error: %v", splitErr),
	})

	merged = appendDiagnosticLine(merged, OutputLine{
		Stream: StreamStderr,
		Text:   "--- ffmpeg HLS fallback attempt ---",
	})
	remaining := maxDiagnosticLines - len(merged)
	if remaining < 0 {
		remaining = 0
	}
	if len(ffmpeg) > remaining {
		ffmpeg = ffmpeg[len(ffmpeg)-remaining:]
	}
	merged = appendDiagnosticLines(merged, ffmpeg...)
	return merged
}

func (m *Manager) runPreparedDownload(ctx context.Context, spec CommandSpec, handler ProgressHandler) (DownloadResult, error) {
	return m.runPreparedDownloadObserved(ctx, spec, handler, nil)
}

func (m *Manager) runPreparedDownloadObserved(
	ctx context.Context,
	spec CommandSpec,
	handler ProgressHandler,
	outputHandler LineHandler,
) (DownloadResult, error) {
	result := DownloadResult{}
	err := m.runner.Run(ctx, spec, func(line OutputLine) {
		result.DiagnosticLines = appendDiagnosticLine(result.DiagnosticLines, line)
		if outputHandler != nil {
			outputHandler(line)
		}
		if progress, ok := parseProgressLine(line.Text); ok {
			result.LastProgress = progress
			if handler != nil {
				handler(progress)
			}
			return
		}
		if path, ok := parseFinalPath(line.Text); ok {
			result.FinalPath = path
		}
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return result, fmt.Errorf("다운로드가 취소되었습니다: %w", err)
		}
		return result, classifyDownloadFailure(sanitizeDownloadError(err))
	}
	if result.FinalPath == "" {
		return result, fmt.Errorf("다운로드는 완료되었지만 최종 파일 경로를 확인할 수 없습니다")
	}
	if result.LastProgress.Percent < 100 {
		result.LastProgress.Percent = 100
	}
	result.LastProgress.Status = "completed"
	if handler != nil {
		handler(result.LastProgress)
	}
	return result, nil
}

func sanitizeDownloadError(err error) error {
	var processErr *ProcessError
	if !errors.As(err, &processErr) {
		return err
	}

	filtered := make([]string, 0, len(processErr.StderrTail))
	for _, line := range processErr.StderrTail {
		if _, ok := parseProgressLine(line); ok {
			continue
		}
		if _, ok := parseFinalPath(line); ok {
			continue
		}
		filtered = append(filtered, line)
	}

	copyErr := *processErr
	copyErr.StderrTail = filtered
	return &copyErr
}
