package downloader

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const sourceURLFixture = `<?xml version="1.0" encoding="UTF-8"?>
<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" xmlns:nvod="urn:naver:vod:2020"><Period>
<AdaptationSet mimeType="video/mp2t">
  <Representation height="1080" bandwidth="8204000" codecs="avc1.64002a,mp4a.40.2" nvod:m3u="https://b01-kr-naver-vod.pstatic.net/1080.m3u8?signature=sensitive" />
  <Representation height="720" bandwidth="3197000" codecs="avc1.4d4020,mp4a.40.2" nvod:m3u="https://b01-kr-naver-vod.pstatic.net/720.m3u8?signature=sensitive" />
  <Representation height="144" bandwidth="173000" codecs="avc1.4dc00c,mp4a.40.2" nvod:m3u="https://b01-kr-naver-vod.pstatic.net/144.m3u8?signature=sensitive" />
</AdaptationSet>
<AdaptationSet mimeType="audio/mp4"><Representation codecs="mp4a.40.2"><BaseURL>https://example.org/track.m4a</BaseURL>
<SegmentList><Initialization range="0-12370"/><SegmentURL mediaRange="12371-78115"/></SegmentList>
</Representation></AdaptationSet></Period></MPD>`

func TestValidSourceURLVideoIDAcceptsCHZZKPlaybackID(t *testing.T) {
	tests := []struct {
		videoID string
		want    bool
	}{
		{videoID: "AAE9A5EEBB58A65EAD9362EE1B296CDDB4EC", want: true}, // Actual 36-char HAR video ID.
		{videoID: strings.Repeat("a", 32), want: true}, // Preserve existing supported length.
		{videoID: strings.Repeat("B", 64), want: true},
		{videoID: strings.Repeat("0", 31), want: false},
		{videoID: strings.Repeat("0", 65), want: false},
		{videoID: strings.Repeat("G", 36), want: false},
		{videoID: strings.Repeat("A", 35) + "/", want: false},
		{videoID: strings.Repeat("A", 35) + "?", want: false},
		{videoID: strings.Repeat("A", 35) + " ", want: false},
		{videoID: "", want: false},
	}
	for _, tt := range tests {
		if got := validSourceURLVideoID(tt.videoID); got != tt.want {
			t.Errorf("video ID length %d: got %t, want %t", len(tt.videoID), got, tt.want)
		}
	}
}

func TestSelectSourceURLHLSIgnoresMissingInitializationSourceURL(t *testing.T) {
	tests := []struct {
		selector, suffix string
		height           int
	}{
		{"bv*+ba/b", "/1080.m3u8", 1080},
		{"bv*[height<=720]+ba/b[height<=720]", "/720.m3u8", 720},
		{"bv*[height<=480]+ba/b[height<=480]", "/144.m3u8", 144},
	}
	for _, tt := range tests {
		url, height, err := selectSourceURLHLS([]byte(sourceURLFixture), tt.selector)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(url, tt.suffix+"?signature=") || height != tt.height {
			t.Fatalf("selector %q: url=%q, height=%d", tt.selector, url, height)
		}
	}
}

func TestSourceURLHLSRejectsUnsafeCandidates(t *testing.T) {
	for _, url := range []string{"http://b01-kr-naver-vod.pstatic.net/a.m3u8", "https://evil.example/a.m3u8", "https://pstatic.net.evil.example/a.m3u8", "https://b01-kr-naver-vod.pstatic.net/a.mp4", "https://user:password@pstatic.net/a.m3u8"} {
		if validSourceURLHLSAddress(url) {
			t.Fatalf("accepted unsafe url: %s", url)
		}
	}
	if _, _, err := selectSourceURLHLS([]byte(sourceURLFixture), "bv*[height<=100]+ba/b[height<=100]"); err == nil {
		t.Fatal("expected unavailable resolution error")
	}
}

func TestBuildSourceURLFFmpegCommandUsesVideoAudioNoOverwrite(t *testing.T) {
	status := ToolchainStatus{}
	status.FFmpeg.Available = true
	status.FFmpeg.Path = "ffmpeg.exe"
	spec, err := buildSourceURLFFmpegCommand(status, DownloadRequest{OutputFormat: "mp4"}, sourceURLHLSStream{URL: "https://cdn.pstatic.net/playlist.m3u8?signature=sensitive", PlaylistPath: filepath.Join(t.TempDir(), "playlist.m3u8")}, "movie.mp4")
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Join(spec.Args, " ")
	for _, fragment := range []string{"-n", "-map 0:v:0", "-map 0:a:0?", "-c copy", "-progress pipe:1", "-protocol_whitelist file,http,https,tcp,tls,crypto", "-seg_max_retry 3", "-f mp4", "movie.mp4"} {
		if !strings.Contains(args, fragment) {
			t.Fatalf("missing argument %s: %s", fragment, args)
		}
	}
	if strings.Contains(args, "signature=sensitive") {
		t.Fatal("signed CDN URL leaked to FFmpeg command")
	}
	if !strings.HasSuffix(args, "movie.mp4") {
		t.Fatal("output path must be the last argument")
	}
}

func TestSourceURLFallbackFileNameSanitizesWindowsPath(t *testing.T) {
	filename := sourceURLFallbackFileName(`a:bad/<name>?`, 15651133, "mp4")
	if filename != "a_bad__name__ [15651133].mp4" {
		t.Fatalf("unexpected filename: %q", filename)
	}
}

func TestClassifySourceURLFailure(t *testing.T) {
	err := classifyDownloadFailure(&ProcessError{Program: "yt-dlp.exe", ExitCode: 1, StderrTail: []string{"KeyError('sourceURL')"}})
	if !isDownloadFailureKind(err, DownloadFailureSourceURLMissing) {
		t.Fatalf("unexpected failure: %v", err)
	}
}

func TestSourceURLMonitorTimeoutIsNotAnAppInterruption(t *testing.T) {
	failed := sourceURLMonitorFailure(
		fallbackMonitorStopDownloadStalled,
		context.Canceled,
		DownloadProgress{Status: progressStatusFallbackDownloading, Percent: 50.9, DownloadedBytes: 2003304496},
	)
	if failed.Kind != DownloadFailureHLSStalled {
		t.Fatalf("unexpected stop kind: %v", failed)
	}
	if errors.Is(failed, context.Canceled) {
		t.Fatalf("internal monitor timeout must not be classified as user cancellation: %v", failed)
	}
	if !strings.Contains(failed.Error(), "180초") || !strings.Contains(failed.Cause.Error(), "50.9%") {
		t.Fatalf("timeout diagnostic missing: %v", failed)
	}
}

func TestSourceURLMonitorReportsIndependentStallTimeout(t *testing.T) {
	previousPoll := ffmpegFallbackPollInterval
	ffmpegFallbackPollInterval = 5 * time.Millisecond
	defer func() { ffmpegFallbackPollInterval = previousPoll }()

	result, err, reason := runMonitoredHLSFallbackWithStallTimeout(
		context.Background(), t.TempDir(), CommandSpec{}, nil,
		func(ctx context.Context, _ CommandSpec, handler ProgressHandler, _ LineHandler) (DownloadResult, error) {
			handler(DownloadProgress{Status: progressStatusFallbackDownloading, Percent: 50.9, DownloadedBytes: 2003304496})
			<-ctx.Done()
			return DownloadResult{LastProgress: DownloadProgress{Percent: 50.9, DownloadedBytes: 2003304496}}, ctx.Err()
		},
		35*time.Millisecond,
	)
	if reason != fallbackMonitorStopDownloadStalled || !errors.Is(err, context.Canceled) {
		t.Fatalf("watchdog did not recognize a stalled child: reason=%q err=%v", reason, err)
	}
	if result.LastProgress.Percent != 50.9 || result.LastProgress.DownloadedBytes != 2003304496 {
		t.Fatalf("last FFmpeg progress lost: %#v", result.LastProgress)
	}
}

