package downloader

import (
	"context"
	"errors"
	"fmt"
)

type ProgressHandler func(DownloadProgress)

type DownloadResult struct {
	FinalPath       string           `json:"finalPath"`
	LogPath         string           `json:"logPath,omitempty"`
	LastProgress    DownloadProgress `json:"lastProgress"`
	DiagnosticLines []OutputLine     `json:"-"`
}

func (m *Manager) Download(ctx context.Context, request DownloadRequest, handler ProgressHandler) (DownloadResult, error) {
	spec, _, err := m.Prepare(ctx, request)
	if err != nil {
		result := DownloadResult{}
		if logPath, logErr := writeDownloadFailureLog(request, spec, result, err); logErr == nil {
			result.LogPath = logPath
		}
		return result, err
	}
	result, err := m.runPreparedDownload(ctx, spec, handler)
	if err != nil {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			if logPath, logErr := writeDownloadFailureLog(request, spec, result, err); logErr == nil {
				result.LogPath = logPath
			}
		}
		return result, err
	}
	_ = cleanupTemporaryDownloadRequest(request)
	return result, nil
}

func (m *Manager) runPreparedDownload(ctx context.Context, spec CommandSpec, handler ProgressHandler) (DownloadResult, error) {
	result := DownloadResult{}
	err := m.runner.Run(ctx, spec, func(line OutputLine) {
		result.DiagnosticLines = appendDiagnosticLine(result.DiagnosticLines, line)
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
