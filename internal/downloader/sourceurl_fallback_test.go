package downloader

import (
	"strings"
	"testing"
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
	spec, err := buildSourceURLFFmpegCommand(status, DownloadRequest{OutputFormat: "mp4"}, sourceURLHLSStream{URL: "https://cdn.pstatic.net/playlist.m3u8?signature=sensitive"}, "movie.mp4")
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Join(spec.Args, " ")
	for _, fragment := range []string{"-n", "-map 0:v:0", "-map 0:a:0?", "-c copy", "-progress pipe:1", "-f mp4", "movie.mp4"} {
		if !strings.Contains(args, fragment) {
			t.Fatalf("missing argument %s: %s", fragment, args)
		}
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
