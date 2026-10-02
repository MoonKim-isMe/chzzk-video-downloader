package downloader

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
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
	ffmpegFallbackPollInterval = time.Second
	ffmpegFallbackStallTimeout = 60 * time.Second
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
) (DownloadResult, CommandSpec, error) {
	firstResult, firstErr := run(ctx, initialSpec, handler, nil)
	if firstErr == nil || !isDownloadFailureKind(firstErr, DownloadFailureHLSInitializationFragmentOrder) {
		return firstResult, initialSpec, firstErr
	}
	if err := ctx.Err(); err != nil {
		return firstResult, initialSpec, fmt.Errorf("다운로드가 취소되었습니다: %w", err)
	}

	fallbackRunID := strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
	tempDir, err := fallbackTemporaryDownloadDirForRequest(request, fallbackRunID)
	if err != nil {
		return firstResult, initialSpec, &DownloadFailure{
			Kind:    DownloadFailureHLSInitializationFragmentOrder,
			Message: "영상 스트림 구조 문제를 감지했지만 대체 다운로드를 준비하지 못했습니다.",
			Cause:   err,
		}
	}
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		return firstResult, initialSpec, &DownloadFailure{
			Kind:    DownloadFailureHLSInitializationFragmentOrder,
			Message: "영상 스트림 구조 문제를 감지했지만 대체 다운로드를 준비하지 못했습니다.",
			Cause:   fmt.Errorf("다운로드 임시 폴더를 만들 수 없습니다: %w", err),
		}
	}

	fallbackSpec, err := buildDownloadCommandWithTempDir(toolchain, request, true, tempDir)
	if err != nil {
		return firstResult, initialSpec, &DownloadFailure{
			Kind:    DownloadFailureHLSInitializationFragmentOrder,
			Message: "영상 스트림 구조 문제를 감지했지만 대체 다운로드를 준비하지 못했습니다.",
			Cause:   err,
		}
	}

	fallbackResult, fallbackErr, stalled := runMonitoredHLSFallback(
		ctx,
		tempDir,
		fallbackSpec,
		handler,
		run,
	)
	if stalled {
		stallSeconds := int(ffmpegFallbackStallTimeout.Seconds())
		fallbackResult.DiagnosticLines = appendDiagnosticLine(
			fallbackResult.DiagnosticLines,
			OutputLine{
				Stream: StreamStderr,
				Text: fmt.Sprintf(
					"fallback monitor: no process output or temporary file growth for %d seconds",
					stallSeconds,
				),
			},
		)
	}
	fallbackResult.DiagnosticLines = mergeHLSFallbackDiagnostics(
		firstResult.DiagnosticLines,
		firstErr,
		fallbackResult.DiagnosticLines,
	)

	if stalled {
		stallSeconds := int(ffmpegFallbackStallTimeout.Seconds())
		return fallbackResult, fallbackSpec, &DownloadFailure{
			Kind:    DownloadFailureHLSInitializationFragmentOrder,
			Message: fmt.Sprintf(
				"대체 다운로드 방식이 %d초 동안 진행되지 않아 중단했습니다.",
				stallSeconds,
			),
			Cause: fmt.Errorf(
				"ffmpeg HLS fallback stalled without output or file growth for %d seconds",
				stallSeconds,
			),
		}
	}
	if fallbackErr != nil {
		if errors.Is(fallbackErr, context.Canceled) || errors.Is(fallbackErr, context.DeadlineExceeded) {
			return fallbackResult, fallbackSpec, fallbackErr
		}
		return fallbackResult, fallbackSpec, &DownloadFailure{
			Kind:    DownloadFailureHLSInitializationFragmentOrder,
			Message: "영상 스트림 구조 문제로 대체 다운로드 방식까지 시도했지만 실패했습니다.",
			Cause:   fallbackErr,
		}
	}
	return fallbackResult, fallbackSpec, nil
}

func runMonitoredHLSFallback(
	ctx context.Context,
	tempDir string,
	spec CommandSpec,
	handler ProgressHandler,
	run downloadAttemptRunner,
) (DownloadResult, error, bool) {
	fallbackCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	type outcome struct {
		result DownloadResult
		err    error
	}

	outcomes := make(chan outcome, 1)
	activity := make(chan struct{}, 1)
	markActivity := func() {
		select {
		case activity <- struct{}{}:
		default:
		}
	}

	if handler != nil {
		handler(DownloadProgress{Status: progressStatusFallbackPreparing})
	}

	go func() {
		result, err := run(
			fallbackCtx,
			spec,
			func(progress DownloadProgress) {
				markActivity()
				if progress.Status != "completed" {
					progress.Status = progressStatusFallbackDownloading
				}
				if handler != nil {
					handler(progress)
				}
			},
			func(OutputLine) {
				markActivity()
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

	lastActivity := time.Now()
	lastSample := lastActivity
	lastBytes, _ := temporaryDownloadSize(tempDir)
	stalled := false
	reportedProcessActivity := false

	for {
		select {
		case current := <-outcomes:
			return current.result, current.err, stalled

		case <-ctx.Done():
			cancel()
			current := <-outcomes
			return current.result, current.err, false

		case <-activity:
			lastActivity = time.Now()
			if !reportedProcessActivity {
				reportedProcessActivity = true
				if handler != nil {
					handler(DownloadProgress{
						Status:          progressStatusFallbackDownloading,
						DownloadedBytes: lastBytes,
					})
				}
			}

		case now := <-ticker.C:
			currentBytes, sizeErr := temporaryDownloadSize(tempDir)
			if sizeErr == nil && currentBytes != lastBytes {
				elapsed := now.Sub(lastSample).Seconds()
				speed := 0.0
				if currentBytes > lastBytes && elapsed > 0 {
					speed = float64(currentBytes-lastBytes) / elapsed
				}

				lastActivity = now
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

			if !stalled &&
				ffmpegFallbackStallTimeout > 0 &&
				now.Sub(lastActivity) >= ffmpegFallbackStallTimeout {
				stalled = true
				cancel()
			}
		}
	}
}

func mergeHLSFallbackDiagnostics(
	first []OutputLine,
	firstErr error,
	second []OutputLine,
) []OutputLine {
	const firstLimit = 190

	merged := make([]OutputLine, 0, maxDiagnosticLines)
	merged = append(merged, OutputLine{
		Stream: StreamStderr,
		Text:   "--- native HLS attempt ---",
	})
	if len(first) > firstLimit {
		first = first[len(first)-firstLimit:]
	}
	merged = append(merged, first...)

	rawFirstErr := firstErr
	if unwrapped := errors.Unwrap(firstErr); unwrapped != nil {
		rawFirstErr = unwrapped
	}
	merged = append(merged, OutputLine{
		Stream: StreamStderr,
		Text:   fmt.Sprintf("native HLS error: %v", rawFirstErr),
	})
	merged = append(merged, OutputLine{
		Stream: StreamStderr,
		Text:   "--- ffmpeg HLS fallback attempt ---",
	})

	remaining := maxDiagnosticLines - len(merged)
	if remaining < 0 {
		remaining = 0
	}
	if len(second) > remaining {
		second = second[len(second)-remaining:]
	}
	merged = append(merged, second...)
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
