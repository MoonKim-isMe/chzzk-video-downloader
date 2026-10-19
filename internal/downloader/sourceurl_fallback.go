package downloader

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MoonKim-isMe/chzzk-video-downloader/internal/chzzk"
)

const (
	sourceURLPlaybackEndpoint = "https://play.vod.naver.com/v3/playback/"
	// A brief stalled segment should not prematurely cancel a large VOD.
	sourceURLHLSStallTimeout = 180 * time.Second
)

var (
	sourceURLHeightLimit    = regexp.MustCompile(`\[height<=([0-9]+)\]`)
	sourceURLVideoIDPattern = regexp.MustCompile(`^[0-9A-Fa-f]{32,64}$`)
)

type sourceURLHLSStream struct {
	URL          string
	PlaylistPath string
	Title        string
	VideoNo      int64
	Height       int
	Duration     int64
}

type sourceURLManifest struct {
	Periods []struct {
		Adaptations []struct {
			MimeType        string `xml:"mimeType,attr"`
			Representations []struct {
				Height    int        `xml:"height,attr"`
				Bandwidth int64      `xml:"bandwidth,attr"`
				Codecs    string     `xml:"codecs,attr"`
				Attrs     []xml.Attr `xml:",any,attr"`
			} `xml:"Representation"`
		} `xml:"AdaptationSet"`
	} `xml:"Period"`
}

func maxHeightFromFormatSelector(selector string) (int, error) {
	matches := sourceURLHeightLimit.FindStringSubmatch(selector)
	if len(matches) == 0 {
		if strings.TrimSpace(selector) == "" || strings.TrimSpace(selector) == DefaultFormatSelector {
			return 0, nil
		}
		return 0, fmt.Errorf("대체 다운로드에서는 현재 화질 선택식을 지원하지 않습니다")
	}
	height, err := strconv.Atoi(matches[1])
	if err != nil || height <= 0 {
		return 0, fmt.Errorf("유효하지 않은 화질 제한입니다")
	}
	return height, nil
}

func selectSourceURLHLS(manifest []byte, selector string) (string, int, error) {
	maxHeight, err := maxHeightFromFormatSelector(selector)
	if err != nil {
		return "", 0, err
	}
	var mpd sourceURLManifest
	if err := xml.Unmarshal(manifest, &mpd); err != nil {
		return "", 0, fmt.Errorf("CHZZK 재생 정보의 MPD 형식을 해석할 수 없습니다")
	}
	var bestURL string
	var bestHeight int
	var bestBandwidth int64
	for _, period := range mpd.Periods {
		for _, adaptation := range period.Adaptations {
			if adaptation.MimeType != "video/mp2t" {
				continue
			}
			for _, rep := range adaptation.Representations {
				// HAR의 video/mp2t HLS는 영상과 AAC 오디오가 함께 포함된 스트림입니다.
				if !strings.Contains(rep.Codecs, "mp4a") || rep.Height <= 0 || (maxHeight > 0 && rep.Height > maxHeight) {
					continue
				}
				for _, attr := range rep.Attrs {
					if attr.Name.Local != "m3u" || attr.Name.Space != "urn:naver:vod:2020" {
						continue
					}
					if !validSourceURLHLSAddress(attr.Value) {
						continue
					}
					if rep.Height > bestHeight || (rep.Height == bestHeight && rep.Bandwidth > bestBandwidth) {
						bestHeight, bestBandwidth, bestURL = rep.Height, rep.Bandwidth, attr.Value
					}
				}
			}
		}
	}
	if bestURL == "" {
		return "", 0, fmt.Errorf("선택한 화질의 영상·오디오 HLS 스트림을 찾을 수 없습니다")
	}
	return bestURL, bestHeight, nil
}

// CHZZK VOD IDs are opaque hexadecimal identifiers. The HAR for VOD 15651133
// contains a 36-character ID; rejecting anything other than 32 characters
// prevents the fallback from reaching the playback API.
func validSourceURLVideoID(videoID string) bool {
	return sourceURLVideoIDPattern.MatchString(videoID)
}

func validSourceURLHLSAddress(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Fragment != "" || !strings.HasSuffix(strings.ToLower(u.Path), ".m3u8") {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "pstatic.net" || strings.HasSuffix(host, ".pstatic.net")
}

func fetchSourceURLHLS(ctx context.Context, request DownloadRequest, client *http.Client) (sourceURLHLSStream, error) {
	normalized, err := normalizeVideoURL(request.URL)
	if err != nil {
		return sourceURLHLSStream{}, err
	}
	videoNo, err := videoNoFromNormalizedURL(normalized)
	if err != nil {
		return sourceURLHLSStream{}, err
	}

	info, err := chzzk.NewClient().GetVideoPlaybackMetadataWithCookiesFile(ctx, videoNo, request.Authentication.CookiesFilePath)
	if err != nil {
		return sourceURLHLSStream{}, fmt.Errorf("CHZZK 영상 재생 정보를 가져오지 못했습니다: %w", err)
	}
	if info.VideoID == "" || info.InKey == "" {
		return sourceURLHLSStream{}, fmt.Errorf("재생에 필요한 영상 식별 정보가 없습니다")
	}
	if !validSourceURLVideoID(info.VideoID) {
		return sourceURLHLSStream{}, fmt.Errorf("영상 식별 정보가 올바르지 않습니다")
	}

	playbackURL, _ := url.Parse(sourceURLPlaybackEndpoint + info.VideoID)
	q := playbackURL.Query()
	q.Set("key", info.InKey)
	q.Set("sid", "2099")
	q.Set("devt", "html5_pc")
	q.Set("st", "13")
	q.Set("lc", "ko_KR")
	q.Set("cpl", "ko_KR")
	playbackURL.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, playbackURL.String(), nil)
	if err != nil {
		return sourceURLHLSStream{}, fmt.Errorf("재생 정보 요청을 생성할 수 없습니다")
	}
	req.Header.Set("Accept", "application/xml, text/xml, */*")
	req.Header.Set("Origin", "https://chzzk.naver.com")
	req.Header.Set("Referer", normalized)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return sourceURLHLSStream{}, ctx.Err()
		}
		return sourceURLHLSStream{}, fmt.Errorf("재생 API 요청에 실패했습니다: 네트워크 연결을 확인해 주세요")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return sourceURLHLSStream{}, fmt.Errorf("재생 API가 HTTP %d를 반환했습니다", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024+1))
	if err != nil || len(body) > 2*1024*1024 {
		return sourceURLHLSStream{}, fmt.Errorf("MPD 재생 정보를 읽을 수 없습니다")
	}
	playlist, height, err := selectSourceURLHLS(body, request.FormatSelector)
	if err != nil {
		return sourceURLHLSStream{}, err
	}
	return sourceURLHLSStream{URL: playlist, VideoNo: videoNo, Title: info.Title, Duration: info.Duration, Height: height}, nil
}

func sourceURLFallbackFileName(title string, videoNo int64, ext string) string {
	title = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, title)
	title = strings.Trim(strings.TrimSpace(title), ". ")
	if title == "" {
		title = "CHZZK"
	}
	chars := []rune(title)
	if len(chars) > 130 {
		title = string(chars[:130])
	}
	return fmt.Sprintf("%s [%d].%s", title, videoNo, ext)
}

func buildSourceURLFFmpegCommand(toolchain ToolchainStatus, request DownloadRequest, stream sourceURLHLSStream, outputPath string) (CommandSpec, error) {
	if !toolchain.FFmpeg.Available {
		return CommandSpec{}, fmt.Errorf("FFmpeg를 사용할 수 없습니다")
	}
	outputFormat, err := normalizeOutputFormat(request.OutputFormat)
	if err != nil {
		return CommandSpec{}, err
	}
	if outputFormat == "webm" {
		return CommandSpec{}, fmt.Errorf("HLS 대체 다운로드는 WebM 무손실 변환을 지원하지 않습니다")
	}
	if outputFormat == "" {
		outputFormat = "mp4"
	}
	format := "mp4"
	if outputFormat == "mkv" {
		format = "matroska"
	}
	if strings.TrimSpace(stream.PlaylistPath) == "" || !filepath.IsAbs(stream.PlaylistPath) {
		return CommandSpec{}, fmt.Errorf("서명을 적용한 로컬 HLS 재생목록이 필요합니다")
	}
	args := []string{
		"-hide_banner", "-loglevel", "error", "-nostdin", "-n",
		// The input is a local manifest; HTTP-only -headers/-user_agent options
		// would fail at open time with "Option not found" on the file protocol.
		"-protocol_whitelist", "file,http,https,tcp,tls,crypto",
		"-seg_max_retry", "3",
		"-i", stream.PlaylistPath,
		"-map", "0:v:0", "-map", "0:a:0?", "-c", "copy",
		"-stats_period", "0.5", "-progress", "pipe:1", "-nostats",
	}
	if outputFormat == "mp4" {
		args = append(args, "-movflags", "+faststart")
	}
	args = append(args, "-f", format, outputPath)
	return CommandSpec{Path: toolchain.FFmpeg.Path, Args: args}, nil
}

func (m *Manager) executeSourceURLFallback(ctx context.Context, request DownloadRequest, toolchain ToolchainStatus, handler ProgressHandler, original DownloadResult, originalErr error) (DownloadResult, CommandSpec, error) {
	source, err := fetchSourceURLHLS(ctx, request, &http.Client{Timeout: 20 * time.Second})
	if err != nil {
		return original, CommandSpec{}, &DownloadFailure{Kind: DownloadFailureSourceURLMissing, Message: "CHZZK 대체 재생 정보를 확인하지 못했습니다.", Cause: err}
	}
	tempDir, err := fallbackTemporaryDownloadDirForRequest(request, "sourceurl-"+strconv.FormatInt(time.Now().UnixNano(), 10))
	if err != nil {
		return original, CommandSpec{}, err
	}
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		return original, CommandSpec{}, err
	}
	manifestDir := filepath.Join(tempDir, "signed-hls")
	// The local HLS playlist contains ephemeral playback signatures.
	defer os.RemoveAll(manifestDir)
	client := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || !validSignedHLSResource(req.URL) {
			return fmt.Errorf("허용되지 않은 HLS 재생목록 이동입니다")
		}
		return nil
	}}
	source.PlaylistPath, err = prepareSignedHLSPlaylist(ctx, client, source.URL, manifestDir)
	if err != nil {
		if ctx.Err() != nil {
			return original, CommandSpec{}, ctx.Err()
		}
		return original, CommandSpec{}, &DownloadFailure{Kind: DownloadFailureSourceURLMissing, Message: "HLS 세그먼트 인증 정보를 준비하지 못했습니다.", Cause: err}
	}
	format, err := normalizeOutputFormat(request.OutputFormat)
	if err != nil {
		return original, CommandSpec{}, err
	}
	if format == "" {
		format = "mp4"
	}
	path := filepath.Join(tempDir, sourceURLFallbackFileName(source.Title, source.VideoNo, format))
	spec, err := buildSourceURLFFmpegCommand(toolchain, request, source, path)
	if err != nil {
		return original, spec, err
	}
	// 완료 파일을 덮어쓰지 않는 기존 정책을 보존합니다.
	destination := filepath.Join(request.OutputDir, filepath.Base(path))
	if _, err := os.Stat(destination); err == nil {
		return original, spec, fmt.Errorf("동일한 다운로드 파일이 이미 존재합니다")
	} else if !os.IsNotExist(err) {
		return original, spec, err
	}

	var mu sync.Mutex
	var latestPercent float64
	run := func(runCtx context.Context, spec CommandSpec, progressHandler ProgressHandler, lineHandler LineHandler) (DownloadResult, error) {
		result := DownloadResult{}
		var seconds float64
		var processedBytes int64
		err := m.runner.Run(runCtx, spec, func(line OutputLine) {
			// Keep FFmpeg errors and significant progress fields rather than
			// filling the bounded diagnostic buffer with frames/fps counters.
			if line.Stream == StreamStderr ||
				strings.HasPrefix(line.Text, "out_time=") ||
				strings.HasPrefix(line.Text, "total_size=") ||
				strings.HasPrefix(line.Text, "progress=") {
				result.DiagnosticLines = appendDiagnosticLine(result.DiagnosticLines, line)
			}
			if lineHandler != nil {
				lineHandler(line)
			}
			if strings.HasPrefix(line.Text, "total_size=") {
				if size, e := strconv.ParseInt(strings.TrimPrefix(line.Text, "total_size="), 10, 64); e == nil && size >= 0 {
					processedBytes = size
				}
			}
			if strings.HasPrefix(line.Text, "out_time_us=") || strings.HasPrefix(line.Text, "out_time_ms=") {
				raw := strings.SplitN(line.Text, "=", 2)[1]
				micros, e := strconv.ParseFloat(raw, 64)
				if e == nil {
					seconds = micros / 1000000
				}
			}
			if strings.HasPrefix(line.Text, "progress=") {
				pct := float64(0)
				if source.Duration > 0 {
					pct = seconds / float64(source.Duration) * 100
				}
				if pct < 0 {
					pct = 0
				}
				if pct > 99.9 {
					pct = 99.9
				}
				result.LastProgress = DownloadProgress{
					Status: progressStatusFallbackDownloading,
					Percent: pct,
					DownloadedBytes: processedBytes,
				}
				if progressHandler != nil {
					progressHandler(result.LastProgress)
				}
			}
		})
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return result, err
			}
			return result, fmt.Errorf("FFmpeg HLS 대체 다운로드에 실패했습니다")
		}
		stat, statErr := os.Stat(path)
		if statErr != nil || stat.Size() == 0 {
			return result, fmt.Errorf("FFmpeg 다운로드 결과 파일이 생성되지 않았습니다")
		}
		result.FinalPath = path
		result.LastProgress = DownloadProgress{Status: "completed", Percent: 100, DownloadedBytes: stat.Size()}
		if progressHandler != nil {
			progressHandler(result.LastProgress)
		}
		return result, nil
	}
	result, runErr, reason := runMonitoredHLSFallbackWithStallTimeout(ctx, tempDir, spec, func(p DownloadProgress) {
		mu.Lock()
		if p.Percent > latestPercent {
			latestPercent = p.Percent
		} else if p.Status != "completed" {
			p.Percent = latestPercent
		}
		mu.Unlock()
		if handler != nil {
			handler(p)
		}
	}, run, sourceURLHLSStallTimeout)
	result.DiagnosticLines = mergeSourceURLFallbackDiagnostics(original.DiagnosticLines, originalErr, result.DiagnosticLines)
	if reason != fallbackMonitorStopNone {
		failure := sourceURLMonitorFailure(reason, runErr, result.LastProgress)
		result.DiagnosticLines = appendDiagnosticLine(result.DiagnosticLines, OutputLine{
			Stream: StreamStderr,
			Text: fmt.Sprintf("HLS fallback monitor: reason=%s progress=%.1f%% bytes=%d timeout_seconds=%d",
				reason, result.LastProgress.Percent, result.LastProgress.DownloadedBytes, int(sourceURLHLSStallTimeout.Seconds())),
		})
		return result, spec, failure
	}
	if runErr != nil {
		return result, spec, &DownloadFailure{Kind: DownloadFailureSourceURLMissing, Message: "FFmpeg HLS 대체 다운로드에 실패했습니다.", Cause: runErr}
	}
	destination, err = promoteFallbackDownload(tempDir, request.OutputDir, result.FinalPath)
	if err != nil {
		return result, spec, &DownloadFailure{Kind: DownloadFailureSourceURLMissing, Message: "다운로드한 파일을 최종 폴더로 이동하지 못했습니다.", Cause: err}
	}
	result.FinalPath = destination
	return result, spec, nil
}

// A child watchdog cancels its own context, but this is not an app/user
// cancellation. Use a non-wrapping cause so the queue preserves the actual
// HLS stall error instead of reclassifying it as "interrupted".
func sourceURLMonitorFailure(reason fallbackMonitorStopReason, runErr error, last DownloadProgress) *DownloadFailure {
	switch reason {
	case fallbackMonitorStopDownloadStalled:
		return &DownloadFailure{
			Kind: DownloadFailureHLSStalled,
			Message: fmt.Sprintf("HLS 대체 다운로드가 %d초 동안 진행되지 않아 중단되었습니다.", int(sourceURLHLSStallTimeout.Seconds())),
			Cause: fmt.Errorf("HLS 감시기: 파일 크기 증가 없음, 진행률 %.1f%%, 기록 크기 %d바이트, FFmpeg 종료 결과: %v",
				last.Percent, last.DownloadedBytes, runErr),
		}
	case fallbackMonitorStopPreparationTimeout:
		return &DownloadFailure{
			Kind: DownloadFailureHLSStalled,
			Message: "FFmpeg HLS 대체 다운로드를 시작하지 못해 중단되었습니다.",
			Cause: fmt.Errorf("HLS 감시기: 다운로드 준비 시간 초과, FFmpeg 종료 결과: %v", runErr),
		}
	default:
		return &DownloadFailure{
			Kind: DownloadFailureSourceURLMissing,
			Message: "FFmpeg HLS 대체 다운로드의 진행 상태를 확인하지 못했습니다.",
			Cause: fmt.Errorf("HLS 감시기: %s, FFmpeg 종료 결과: %v", reason, runErr),
		}
	}
}

func mergeSourceURLFallbackDiagnostics(original []OutputLine, firstErr error, fallback []OutputLine) []OutputLine {
	lines := make([]OutputLine, 0, len(original)+len(fallback)+2)
	for _, line := range original {
		lines = appendDiagnosticLine(lines, line)
	}
	lines = appendDiagnosticLine(lines, OutputLine{Stream: StreamStderr, Text: "--- sourceURL HLS fallback ---"})
	if firstErr != nil {
		lines = appendDiagnosticLine(lines, OutputLine{Stream: StreamStderr, Text: redactDiagnosticText(firstErr.Error())})
	}
	for _, line := range fallback {
		lines = appendDiagnosticLine(lines, line)
	}
	return lines
}
