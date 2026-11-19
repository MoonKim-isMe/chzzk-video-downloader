package downloader

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Some CHZZK playback manifests expose distinct byte-range MP4 video and
// audio resources instead of video/mp2t HLS. These BaseURLs represent complete
// media objects; FFmpeg remuxes their locally downloaded tracks without using
// signed CDN URLs in subprocess arguments.
func selectSourceURLDASHVideo(manifest []byte, selector string) (string, string, int, error) {
	maxHeight, err := maxHeightFromFormatSelector(selector)
	if err != nil {
		return "", "", 0, err
	}
	var mpd sourceURLManifest
	if err := xml.Unmarshal(manifest, &mpd); err != nil {
		return "", "", 0, fmt.Errorf("MP4 영상 재생 정보를 해석할 수 없습니다")
	}
	var videoURL, videoCodecs string
	var videoHeight int
	var videoBandwidth int64
	for _, period := range mpd.Periods {
		for _, adaptation := range period.Adaptations {
			for _, rep := range adaptation.Representations {
				mimeType := adaptation.MimeType
				if mimeType == "" {
					mimeType = rep.MimeType
				}
				if !strings.EqualFold(mimeType, "video/mp4") {
					continue
				}
				codecs := rep.Codecs
				if codecs == "" {
					codecs = adaptation.Codecs
				}
				codecs = strings.ToLower(codecs)
				if !strings.Contains(codecs, "avc1") && !strings.Contains(codecs, "av01") &&
					!strings.Contains(codecs, "hvc1") && !strings.Contains(codecs, "hev1") {
					continue
				}
				if rep.Height <= 0 || (maxHeight > 0 && rep.Height > maxHeight) {
					continue
				}
				address := strings.TrimSpace(rep.BaseURL)
				if address == "" {
					address = strings.TrimSpace(adaptation.BaseURL)
				}
				if !validSourceURLDASHAudioAddress(address) {
					continue
				}
				if videoURL == "" || rep.Height > videoHeight ||
					(rep.Height == videoHeight && rep.Bandwidth > videoBandwidth) {
					videoURL, videoCodecs = address, codecs
					videoHeight, videoBandwidth = rep.Height, rep.Bandwidth
				}
			}
		}
	}
	if videoURL == "" {
		return "", "", 0, fmt.Errorf("선택한 화질의 MP4 영상 원본 주소를 찾을 수 없습니다")
	}
	// A muxed video/mp4 representation can already contain its own AAC track.
	if strings.Contains(videoCodecs, "mp4a") {
		return videoURL, "", videoHeight, nil
	}
	audioURL, err := selectSourceURLDASHAudio(manifest)
	if err != nil {
		return "", "", 0, fmt.Errorf("MP4 영상에 결합할 오디오 원본을 찾을 수 없습니다: %w", err)
	}
	return videoURL, audioURL, videoHeight, nil
}

func buildSourceURLDASHVideoFFmpegCommand(
	toolchain ToolchainStatus, request DownloadRequest,
	videoPath, audioPath, outputPath string,
) (CommandSpec, error) {
	if !toolchain.FFmpeg.Available {
		return CommandSpec{}, fmt.Errorf("FFmpeg를 사용할 수 없습니다")
	}
	if NormalizeDownloadType(request.DownloadType) != DownloadTypeVideo ||
		!filepath.IsAbs(videoPath) || !filepath.IsAbs(outputPath) ||
		(audioPath != "" && !filepath.IsAbs(audioPath)) {
		return CommandSpec{}, fmt.Errorf("MP4 영상 대체 다운로드 경로가 올바르지 않습니다")
	}
	format, err := sourceURLFallbackOutputFormat(request)
	if err != nil {
		return CommandSpec{}, err
	}
	if !strings.EqualFold(filepath.Ext(outputPath), "."+format) {
		return CommandSpec{}, fmt.Errorf("영상 결과 파일의 확장자가 올바르지 않습니다")
	}
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-n"}
	if request.Segment != nil {
		args = append(args, "-ss", ffmpegTimestamp(request.Segment.StartSeconds))
	}
	args = append(args, "-i", videoPath)
	if audioPath != "" {
		if request.Segment != nil {
			args = append(args, "-ss", ffmpegTimestamp(request.Segment.StartSeconds))
		}
		args = append(args, "-i", audioPath)
	}
	if request.Segment != nil {
		if end, finite := segmentEndExclusive(request.Segment); finite {
			args = append(args, "-t", strconv.FormatInt(end-request.Segment.StartSeconds, 10))
		}
	}
	args = append(args, "-map", "0:v:0")
	if audioPath != "" {
		args = append(args, "-map", "1:a:0")
	} else {
		args = append(args, "-map", "0:a:0")
	}
	args = append(args, "-c", "copy", "-stats_period", "0.5", "-progress", "pipe:1", "-nostats")
	if format == "mp4" {
		args = append(args, "-movflags", "+faststart")
	}
	if format == "mkv" {
		format = "matroska"
	}
	args = append(args, "-f", format, outputPath)
	return CommandSpec{Path: toolchain.FFmpeg.Path, Args: args}, nil
}

// This is intentionally separate from the HLS segment cache. The whole-file
// DASH path does not yet implement byte-range resume or parallel acceleration.
func (m *Manager) executeSourceURLDASHVideoFallback(
	ctx context.Context, request DownloadRequest, toolchain ToolchainStatus,
	handler ProgressHandler, original DownloadResult, originalErr error,
	source sourceURLHLSStream,
) (DownloadResult, CommandSpec, error) {
	if source.DASHVideoURL == "" {
		return original, CommandSpec{}, fmt.Errorf("MP4 영상 주소가 제공되지 않았습니다")
	}
	tempDir, err := fallbackTemporaryDownloadDirForRequest(request,
		"dash-video-"+strconv.FormatInt(time.Now().UnixNano(), 10))
	if err != nil {
		return original, CommandSpec{}, err
	}
	if err := os.MkdirAll(tempDir, 0700); err != nil {
		return original, CommandSpec{}, err
	}
	defer os.RemoveAll(tempDir)
	format, err := sourceURLFallbackOutputFormat(request)
	if err != nil {
		return original, CommandSpec{}, err
	}
	outputName := sourceURLFallbackFileName(source.Title, source.VideoNo, format)
	if request.Segment != nil {
		outputName = request.Segment.FileName + "." + format
	}
	finalPath := filepath.Join(request.OutputDir, outputName)
	if _, statErr := os.Stat(finalPath); statErr == nil {
		return original, CommandSpec{}, fmt.Errorf("동일한 다운로드 파일이 이미 존재합니다")
	} else if !os.IsNotExist(statErr) {
		return original, CommandSpec{}, statErr
	}
	videoPath := filepath.Join(tempDir, "video-input.mp4")
	audioPath := ""
	if source.DASHAudioURL != "" {
		audioPath = filepath.Join(tempDir, "audio-input.m4a")
	}
	outputPath := filepath.Join(tempDir, outputName)
	spec, err := buildSourceURLDASHVideoFFmpegCommand(toolchain, request, videoPath, audioPath, outputPath)
	if err != nil {
		return original, spec, err
	}
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 || !validSourceURLDASHAudioAddress(req.URL.String()) {
				return fmt.Errorf("허용되지 않은 MP4 미디어 서버 이동입니다")
			}
			return nil
		},
	}
	result := original
	result.DiagnosticLines = mergeSourceURLFallbackDiagnostics(original.DiagnosticLines, originalErr, nil)
	result.DiagnosticLines = appendDiagnosticLine(result.DiagnosticLines, OutputLine{
		Stream: StreamStderr, Text: "HLS 주소 부재: MP4 영상·오디오 원본을 내려받은 뒤 스트림 복사를 시도합니다.",
	})
	report := func(pct float64, bytes int64) {
		result.LastProgress = DownloadProgress{
			Status: progressStatusFallbackDownloading,
			Percent: pct,
			DownloadedBytes: bytes,
		}
		if handler != nil {
			handler(result.LastProgress)
		}
	}
	downloadTrack := func(rawURL, path string, offset, weight float64) error {
		return downloadSourceURLDASHAudio(ctx, client, rawURL, path, func(done, total int64) {
			pct := offset
			if total > 0 {
				pct += float64(done) / float64(total) * weight
			}
			report(pct, done)
		})
	}
	videoWeight := float64(65)
	if audioPath == "" {
		videoWeight = 70
	}
	if err := downloadTrack(source.DASHVideoURL, videoPath, 0, videoWeight); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return result, spec, err
		}
		return result, spec, &DownloadFailure{Kind: DownloadFailureSourceURLMissing,
			Message: "MP4 영상 원본 다운로드에 실패했습니다.", Cause: err}
	}
	if audioPath != "" {
		if err := downloadTrack(source.DASHAudioURL, audioPath, videoWeight, 5); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return result, spec, err
			}
			return result, spec, &DownloadFailure{Kind: DownloadFailureSourceURLMissing,
				Message: "MP4 오디오 원본 다운로드에 실패했습니다.", Cause: err}
		}
	}
	var seconds float64
	var muxedBytes int64
	err = m.runner.Run(ctx, spec, func(line OutputLine) {
		if line.Stream == StreamStderr ||
			strings.HasPrefix(line.Text, "out_time=") ||
			strings.HasPrefix(line.Text, "total_size=") ||
			strings.HasPrefix(line.Text, "progress=") {
			result.DiagnosticLines = appendDiagnosticLine(result.DiagnosticLines, line)
		}
		if strings.HasPrefix(line.Text, "total_size=") {
			if size, parseErr := strconv.ParseInt(strings.TrimPrefix(line.Text, "total_size="), 10, 64); parseErr == nil {
				muxedBytes = size
			}
		}
		if strings.HasPrefix(line.Text, "out_time_us=") || strings.HasPrefix(line.Text, "out_time_ms=") {
			raw := strings.SplitN(line.Text, "=", 2)[1]
			if micros, parseErr := strconv.ParseFloat(raw, 64); parseErr == nil {
				seconds = micros / 1000000
			}
		}
		if strings.HasPrefix(line.Text, "progress=") {
			duration := float64(source.Duration)
			if request.Segment != nil {
				duration = segmentDuration(request.Segment, source.Duration)
			}
			pct := float64(70)
			if duration > 0 {
				pct += seconds / duration * 29.9
			}
			if pct > 99.9 {
				pct = 99.9
			}
			result.LastProgress = DownloadProgress{
				Status: progressStatusFallbackDownloading, Percent: pct,
				DownloadedBytes: muxedBytes, ProcessedMediaSeconds: seconds,
			}
			if handler != nil {
				handler(result.LastProgress)
			}
		}
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return result, spec, err
		}
		return result, spec, &DownloadFailure{Kind: DownloadFailureSourceURLMissing,
			Message: "MP4 영상·오디오 합치기에 실패했습니다.", Cause: sanitizeDownloadError(err)}
	}
	info, err := os.Stat(outputPath)
	if err != nil || info.Size() == 0 {
		return result, spec, &DownloadFailure{Kind: DownloadFailureSourceURLMissing,
			Message: "MP4 영상 대체 다운로드 결과 파일이 생성되지 않았습니다."}
	}
	destination, err := promoteFallbackDownload(tempDir, request.OutputDir, outputPath)
	if err != nil {
		return result, spec, &DownloadFailure{Kind: DownloadFailureSourceURLMissing,
			Message: "다운로드한 MP4 파일을 최종 폴더로 이동하지 못했습니다.", Cause: err}
	}
	result.FinalPath = destination
	result.LastProgress = DownloadProgress{Status: "completed", Percent: 100, DownloadedBytes: info.Size()}
	if handler != nil {
		handler(result.LastProgress)
	}
	return result, spec, nil
}
