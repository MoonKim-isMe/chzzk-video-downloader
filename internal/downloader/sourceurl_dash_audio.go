package downloader

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// MP4 audio is sometimes the only source in a CHZZK MPD. In that case,
// download the signed audio object to a private local file before invoking
// FFmpeg. Never include its signed CDN URL in subprocess arguments or logs.
const maxSourceURLDASHMediaBytes int64 = 64 << 30

func buildSourceURLDASHAudioFFmpegCommand(toolchain ToolchainStatus, request DownloadRequest, inputPath, outputPath string) (CommandSpec, error) {
	if !toolchain.FFmpeg.Available {
		return CommandSpec{}, fmt.Errorf("MP3 변환 기능을 사용할 수 없습니다")
	}
	if NormalizeDownloadType(request.DownloadType) != DownloadTypeAudio ||
		!filepath.IsAbs(inputPath) || !filepath.IsAbs(outputPath) ||
		!strings.EqualFold(filepath.Ext(outputPath), ".mp3") {
		return CommandSpec{}, fmt.Errorf("MP3 대체 다운로드 경로가 올바르지 않습니다")
	}
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-n"}
	if request.Segment != nil {
		args = append(args, "-ss", ffmpegTimestamp(request.Segment.StartSeconds))
	}
	args = append(args, "-i", inputPath)
	if request.Segment != nil {
		if end, finite := segmentEndExclusive(request.Segment); finite {
			args = append(args, "-t", strconv.FormatInt(end-request.Segment.StartSeconds, 10))
		}
	}
	args = append(args, "-map", "0:a:0", "-vn", "-codec:a", "libmp3lame", "-q:a", "0",
		"-stats_period", "0.5", "-progress", "pipe:1", "-nostats", "-f", "mp3", outputPath)
	return CommandSpec{Path: toolchain.FFmpeg.Path, Args: args}, nil
}

// The caller provides a redirect-restricted client. A partial HTTP response
// must never be treated as a complete MP4 input.
func downloadSourceURLDASHAudio(ctx context.Context, client *http.Client, signedURL, outputPath string, progress func(int64, int64)) error {
	if !validSourceURLDASHAudioAddress(signedURL) {
		return fmt.Errorf("허용되지 않은 MP4 미디어 서버입니다")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, signedURL, nil)
	if err != nil {
		return fmt.Errorf("MP4 미디어 요청을 준비할 수 없습니다")
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Origin", "https://chzzk.naver.com")
	req.Header.Set("Referer", "https://chzzk.naver.com/")
	req.Header.Set("Accept", "audio/mp4, video/mp4, */*")
	response, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// net/url.Error can embed the signed URL; never wrap it.
		return fmt.Errorf("MP4 미디어 서버에 연결할 수 없습니다")
	}
	defer response.Body.Close()
	if !validSourceURLDASHAudioAddress(response.Request.URL.String()) {
		return fmt.Errorf("MP4 미디어가 허용되지 않은 서버로 이동했습니다")
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("MP4 미디어 서버가 HTTP %d를 반환했습니다", response.StatusCode)
	}
	if response.ContentLength > maxSourceURLDASHMediaBytes {
		return fmt.Errorf("MP4 미디어 크기가 허용 범위를 초과했습니다")
	}
	file, err := os.OpenFile(outputPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("MP4 미디어 임시 파일을 생성할 수 없습니다: %w", err)
	}
	complete := false
	defer func() {
		_ = file.Close()
		if !complete {
			_ = os.Remove(outputPath)
		}
	}()
	var downloaded int64
	var lastReport time.Time
	buffer := make([]byte, 128*1024)
	for {
		count, readErr := response.Body.Read(buffer)
		if count > 0 {
			downloaded += int64(count)
			if downloaded > maxSourceURLDASHMediaBytes {
				return fmt.Errorf("MP4 미디어 크기가 허용 범위를 초과했습니다")
			}
			if _, writeErr := file.Write(buffer[:count]); writeErr != nil {
				return fmt.Errorf("MP4 미디어 임시 저장에 실패했습니다: %w", writeErr)
			}
			if progress != nil && time.Since(lastReport) >= 300*time.Millisecond {
				progress(downloaded, response.ContentLength)
				lastReport = time.Now()
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("MP4 미디어 수신이 중단되었습니다")
		}
	}
	if downloaded == 0 || (response.ContentLength >= 0 && downloaded != response.ContentLength) {
		return fmt.Errorf("MP4 미디어 파일이 비어 있거나 완전히 수신되지 않았습니다")
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("MP4 미디어 파일을 저장할 수 없습니다: %w", err)
	}
	complete = true
	if progress != nil {
		progress(downloaded, downloaded)
	}
	return nil
}

func (m *Manager) executeSourceURLDASHAudioFallback(ctx context.Context, request DownloadRequest, toolchain ToolchainStatus, handler ProgressHandler, original DownloadResult, originalErr error, source sourceURLHLSStream) (DownloadResult, CommandSpec, error) {
	tempDir, err := fallbackTemporaryDownloadDirForRequest(request, "dash-audio-"+strconv.FormatInt(time.Now().UnixNano(), 10))
	if err != nil {
		return original, CommandSpec{}, err
	}
	if err := os.MkdirAll(tempDir, 0700); err != nil {
		return original, CommandSpec{}, err
	}
	outputName := sourceURLFallbackFileName(source.Title, source.VideoNo, "mp3")
	if request.Segment != nil {
		outputName = request.Segment.FileName + ".mp3"
	}
	outputPath := filepath.Join(tempDir, outputName)
	inputPath := filepath.Join(tempDir, "audio-input.mp4")
	spec, err := buildSourceURLDASHAudioFFmpegCommand(toolchain, request, inputPath, outputPath)
	if err != nil {
		return original, spec, err
	}
	destination := filepath.Join(request.OutputDir, filepath.Base(outputPath))
	if _, statErr := os.Stat(destination); statErr == nil {
		return original, spec, fmt.Errorf("동일한 다운로드 파일이 이미 존재합니다")
	} else if !os.IsNotExist(statErr) {
		return original, spec, statErr
	}
	// Only HTTPS pstatic.net endpoints are allowed, including redirects.
	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || !validSourceURLDASHAudioAddress(req.URL.String()) {
			return fmt.Errorf("허용되지 않은 MP4 오디오 서버 이동입니다")
		}
		return nil
	}}
	result := original
	result.DiagnosticLines = mergeSourceURLFallbackDiagnostics(original.DiagnosticLines, originalErr, nil)
	result.DiagnosticLines = appendDiagnosticLine(result.DiagnosticLines, OutputLine{
		Stream: StreamStderr, Text: "HLS 주소 부재: MP4 오디오 원본 다운로드 및 MP3 변환을 시도합니다.",
	})
	downloadErr := downloadSourceURLDASHAudio(ctx, client, source.DASHAudioURL, inputPath, func(done, total int64) {
		pct := float64(0)
		if total > 0 {
			pct = float64(done) / float64(total) * 45
		}
		result.LastProgress = DownloadProgress{Status: progressStatusFallbackDownloading, Percent: pct, DownloadedBytes: done}
		if handler != nil {
			handler(result.LastProgress)
		}
	})
	if downloadErr != nil {
		if errors.Is(downloadErr, context.Canceled) || errors.Is(downloadErr, context.DeadlineExceeded) {
			return result, spec, downloadErr
		}
		return result, spec, &DownloadFailure{
			Kind: DownloadFailureSourceURLMissing, Message: "MP4 오디오 대체 다운로드에 실패했습니다.", Cause: downloadErr,
		}
	}
	defer os.Remove(inputPath)
	var seconds float64
	var encodedBytes int64
	runErr := m.runner.Run(ctx, spec, func(line OutputLine) {
		if line.Stream == StreamStderr ||
			strings.HasPrefix(line.Text, "out_time=") ||
			strings.HasPrefix(line.Text, "total_size=") ||
			strings.HasPrefix(line.Text, "progress=") {
			result.DiagnosticLines = appendDiagnosticLine(result.DiagnosticLines, line)
		}
		if strings.HasPrefix(line.Text, "out_time_us=") || strings.HasPrefix(line.Text, "out_time_ms=") {
			value := strings.SplitN(line.Text, "=", 2)[1]
			if micros, e := strconv.ParseFloat(value, 64); e == nil {
				seconds = micros / 1000000
			}
		}
		if strings.HasPrefix(line.Text, "total_size=") {
			if size, e := strconv.ParseInt(strings.TrimPrefix(line.Text, "total_size="), 10, 64); e == nil && size >= 0 {
				encodedBytes = size
			}
		}
		if strings.HasPrefix(line.Text, "progress=") {
			pct := float64(45)
			duration := float64(source.Duration)
			if request.Segment != nil {
				duration = segmentDuration(request.Segment, source.Duration)
			}
			if duration > 0 {
				pct += seconds / duration * 54.9
			}
			if pct > 99.9 {
				pct = 99.9
			}
			result.LastProgress = DownloadProgress{Status: progressStatusFallbackDownloading, Percent: pct, ProcessedMediaSeconds: seconds, DownloadedBytes: encodedBytes}
			if handler != nil {
				handler(result.LastProgress)
			}
		}
	})
	if runErr != nil {
		if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) {
			return result, spec, runErr
		}
		return result, spec, &DownloadFailure{
			Kind: DownloadFailureSourceURLMissing, Message: "MP4 오디오의 MP3 변환에 실패했습니다.", Cause: sanitizeDownloadError(runErr),
		}
	}
	stat, err := os.Stat(outputPath)
	if err != nil || stat.Size() == 0 {
		return result, spec, &DownloadFailure{Kind: DownloadFailureSourceURLMissing, Message: "MP3 결과 파일이 생성되지 않았습니다."}
	}
	finalPath, err := promoteFallbackDownload(tempDir, request.OutputDir, outputPath)
	if err != nil {
		return result, spec, &DownloadFailure{Kind: DownloadFailureSourceURLMissing, Message: "MP3 결과 파일을 최종 폴더로 이동하지 못했습니다.", Cause: err}
	}
	result.FinalPath = finalPath
	result.LastProgress = DownloadProgress{Status: "completed", Percent: 100, DownloadedBytes: stat.Size()}
	if handler != nil {
		handler(result.LastProgress)
	}
	return result, spec, nil
}
