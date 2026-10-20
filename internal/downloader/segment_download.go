package downloader

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Segment end is inclusive in the editor. FFmpeg and yt-dlp require an
// exclusive upper bound, so the following whole second is used.
func segmentEndExclusive(segment *DownloadSegment) (int64, bool) {
	if segment == nil || segment.EndSeconds == nil {
		return 0, false
	}
	return *segment.EndSeconds + 1, true
}

func segmentDuration(segment *DownloadSegment, fullDuration int64) float64 {
	if segment == nil {
		return float64(fullDuration)
	}
	end, finite := segmentEndExclusive(segment)
	if !finite {
		end = fullDuration
	}
	if end <= segment.StartSeconds {
		return 0
	}
	return float64(end - segment.StartSeconds)
}

func ffmpegTimestamp(seconds int64) string {
	return fmt.Sprintf("%02d:%02d:%02d", seconds/3600, seconds/60%60, seconds%60)
}

func ytDLPSectionRange(segment *DownloadSegment) (string, string) {
	end, ok := segmentEndExclusive(segment)
	if !ok {
		return ffmpegTimestamp(segment.StartSeconds), "inf"
	}
	return ffmpegTimestamp(segment.StartSeconds), ffmpegTimestamp(end)
}

func segmentFileIdentity(segment *DownloadSegment) string {
	if segment == nil {
		return ""
	}
	end := "end"
	if segment.EndSeconds != nil {
		end = strconv.FormatInt(*segment.EndSeconds, 10)
	}
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s/%d/%d/%s", segment.FileName, segment.Position, segment.StartSeconds, end)))
	return hex.EncodeToString(digest[:8])
}

// A discontinuity recovery may need the complete source because format
// fragments cannot be cut individually on their own timelines. Recover into
// a private work folder and clip the resulting full file with FFmpeg.
// Never copy a whole VOD into the user's clip folder.
func (m *Manager) downloadRecoveredSegment(
	ctx context.Context,
	request DownloadRequest,
	toolchain ToolchainStatus,
	handler ProgressHandler,
) (DownloadResult, error) {
	if request.Segment == nil {
		return DownloadResult{}, fmt.Errorf("구간 다운로드 정보가 필요합니다")
	}
	outputFormat, err := sourceURLFallbackOutputFormat(request)
	if err != nil {
		return DownloadResult{}, err
	}
	if err := os.MkdirAll(request.OutputDir, 0o755); err != nil {
		return DownloadResult{}, err
	}
	workDir, err := os.MkdirTemp(request.OutputDir, ".chzzk-segment-recovery-")
	if err != nil {
		return DownloadResult{}, fmt.Errorf("구간 복구 임시 폴더를 만들 수 없습니다: %w", err)
	}
	defer os.RemoveAll(workDir)

	whole := request
	whole.Segment = nil
	whole.OutputDir = filepath.Join(workDir, "full")
	whole.OutputTemplate = DefaultOutputTemplate
	wholeResult, err := m.Download(ctx, whole, func(p DownloadProgress) {
		if handler == nil {
			return
		}
		// The full-media repair is stage one; transcoding is stage two.
		p.Percent = p.Percent * 0.90
		p.Status = progressStatusFallbackDownloading
		handler(p)
	})
	if err != nil {
		return wholeResult, fmt.Errorf("전체 VOD 복구 후 구간 추출 준비에 실패했습니다: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return wholeResult, err
	}
	source := wholeResult.FinalPath
	if source == "" {
		return wholeResult, fmt.Errorf("복구된 VOD 파일을 찾을 수 없습니다")
	}
	cutDir := filepath.Join(workDir, "clip")
	if err := os.MkdirAll(cutDir, 0o755); err != nil {
		return wholeResult, err
	}
	outputPath := filepath.Join(cutDir, request.Segment.FileName+"."+outputFormat)
	if _, err := os.Stat(filepath.Join(request.OutputDir, filepath.Base(outputPath))); err == nil {
		return wholeResult, fmt.Errorf("동일한 구간 다운로드 파일이 이미 존재합니다")
	} else if !os.IsNotExist(err) {
		return wholeResult, err
	}

	arguments := []string{
		"-hide_banner", "-loglevel", "error", "-nostdin", "-n",
		"-ss", ffmpegTimestamp(request.Segment.StartSeconds),
		"-i", source,
	}
	if end, finite := segmentEndExclusive(request.Segment); finite {
		arguments = append(arguments, "-t", strconv.FormatInt(end-request.Segment.StartSeconds, 10))
	}
	if NormalizeDownloadType(request.DownloadType) == DownloadTypeAudio {
		arguments = append(arguments, "-map", "0:a:0", "-vn", "-c:a", "libmp3lame", "-q:a", "0")
	} else {
		arguments = append(arguments, "-map", "0:v:0", "-map", "0:a:0?")
		arguments = append(arguments, "-c", "copy")
		if outputFormat == "mp4" {
			arguments = append(arguments, "-movflags", "+faststart")
		}
	}
	arguments = append(arguments, "-progress", "pipe:1", "-nostats", outputPath)
	clipDuration := segmentDuration(request.Segment, 0)
	var last DownloadProgress
	clipOutput := func(line OutputLine) {
		if handler == nil || clipDuration <= 0 || !strings.HasPrefix(line.Text, "out_time_us=") {
			return
		}
		micros, parseErr := strconv.ParseInt(strings.TrimPrefix(line.Text, "out_time_us="), 10, 64)
		if parseErr != nil || micros < 0 {
			return
		}
		last = DownloadProgress{Status: progressStatusFallbackDownloading, Percent: 90 + 9.9*min(1, float64(micros)/1e6/clipDuration)}
		handler(last)
	}
	err = m.runner.Run(ctx, CommandSpec{Path: toolchain.FFmpeg.Path, Args: arguments}, clipOutput)
	// Match normal HLS downloads: copy original codecs without an encoder
	// first. Only retry with a supported encoder if remuxing fails.
	if err != nil && NormalizeDownloadType(request.DownloadType) == DownloadTypeVideo && ctx.Err() == nil {
		copyErr := err
		encoder, probeErr := probeSegmentVideoEncoder(ctx, toolchain.FFmpeg.Path, outputFormat)
		if probeErr != nil {
			return wholeResult, fmt.Errorf("복구된 영상의 구간 스트림 복사 실패 (%v), 재인코딩 준비 실패: %w", copyErr, probeErr)
		}
		options, encodeErr := segmentVideoEncodingArgs(outputFormat, encoder)
		if encodeErr != nil {
			return wholeResult, fmt.Errorf("구간 복구 재인코딩 인수 오류: %w", encodeErr)
		}
		if removeErr := os.Remove(outputPath); removeErr != nil && !os.IsNotExist(removeErr) {
			return wholeResult, fmt.Errorf("구간 재시도 전 출력 파일 정리에 실패했습니다: %w", removeErr)
		}
		// Replace -c copy with the codec-specific options. Input, times,
		// output filename and progress reporting remain unchanged.
		updated := false
		for i := 0; i+1 < len(arguments); i++ {
			if arguments[i] == "-c" && arguments[i+1] == "copy" {
				retryArgs := append([]string{}, arguments[:i]...)
				retryArgs = append(retryArgs, options...)
				arguments = append(retryArgs, arguments[i+2:]...)
				updated = true
				break
			}
		}
		if !updated {
			return wholeResult, fmt.Errorf("스트림 복사 인코더 옵션을 찾지 못했습니다")
		}
		err = m.runner.Run(ctx, CommandSpec{Path: toolchain.FFmpeg.Path, Args: arguments}, clipOutput)
		if err != nil {
			return wholeResult, fmt.Errorf("복구된 영상의 구간 스트림 복사 (%v) 및 %s 재인코딩 실패: %w", copyErr, encoder, err)
		}
	}
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return wholeResult, err
		}
		return wholeResult, fmt.Errorf("복구된 영상에서 구간을 추출하지 못했습니다: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return wholeResult, err
	}
	stat, err := os.Stat(outputPath)
	if err != nil || stat.Size() == 0 {
		return wholeResult, fmt.Errorf("잘라낸 구간 파일이 비어 있습니다")
	}
	finalPath, err := promoteFallbackDownload(cutDir, request.OutputDir, outputPath)
	if err != nil {
		return wholeResult, err
	}
	done := DownloadProgress{Status: "completed", Percent: 100, DownloadedBytes: stat.Size()}
	if handler != nil {
		handler(done)
	}
	return DownloadResult{FinalPath: finalPath, LastProgress: done}, nil
}
