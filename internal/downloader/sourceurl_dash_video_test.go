package downloader

import (
	"path/filepath"
	"strings"
	"testing"
)

const sourceURLDASHVideoFixture = `<?xml version="1.0" encoding="UTF-8"?>
<MPD xmlns="urn:mpeg:dash:schema:mpd:2011"><Period>
<AdaptationSet mimeType="video/mp4">
  <Representation height="1080" bandwidth="9000000" codecs="avc1.64002a">
    <BaseURL>https://cdn.pstatic.net/video-1080.mp4?_lsu_sa_=signed</BaseURL>
    <SegmentList><Initialization range="0-100"/><SegmentURL mediaRange="101-500"/></SegmentList>
  </Representation>
  <Representation height="720" bandwidth="3500000" codecs="avc1.4d4020">
    <BaseURL>https://cdn.pstatic.net/video-720.mp4?_lsu_sa_=signed</BaseURL>
  </Representation>
</AdaptationSet>
<AdaptationSet mimeType="audio/mp4">
  <Representation bandwidth="160000" codecs="mp4a.40.2">
    <BaseURL>https://cdn.pstatic.net/audio.m4a?_lsu_sa_=signed</BaseURL>
    <SegmentList><Initialization range="0-100"/><SegmentURL mediaRange="101-200"/></SegmentList>
  </Representation>
</AdaptationSet>
</Period></MPD>`

func TestSelectSourceURLDASHVideoPreservesQualityAndAudio(t *testing.T) {
	for _, tt := range []struct {
		selector string
		height int
		part string
	}{
		{"bv*+ba/b", 1080, "video-1080.mp4"},
		{"bv*[height<=720]+ba/b[height<=720]", 720, "video-720.mp4"},
	} {
		video, audio, height, err := selectSourceURLDASHVideo([]byte(sourceURLDASHVideoFixture), tt.selector)
		if err != nil || height != tt.height ||
			!strings.Contains(video, tt.part) || !strings.Contains(audio, "/audio.m4a") {
			t.Fatalf("quality %q: video=%q audio=%q height=%d err=%v", tt.selector, video, audio, height, err)
		}
	}
	if _, _, err := selectSourceURLHLSForType([]byte(sourceURLDASHVideoFixture),
		"bv*+ba/b", DownloadTypeVideo); err == nil {
		t.Fatal("DASH-only fixture must not expose an HLS video")
	}
}

func TestSelectSourceURLDASHVideoRejectsInvalidSources(t *testing.T) {
	for _, bad := range []string{
		"https://example.org/video-1080.mp4",
		"http://cdn.pstatic.net/video-1080.mp4",
		"https://pstatic.net.evil.example/video-1080.mp4",
		"https://cdn.pstatic.net/video-1080.m3u8",
	} {
		manifest := strings.ReplaceAll(sourceURLDASHVideoFixture,
			"https://cdn.pstatic.net/video-1080.mp4?_lsu_sa_=signed", bad)
		video, _, _, err := selectSourceURLDASHVideo([]byte(manifest), "bv*+ba/b")
		if err != nil || !strings.Contains(video, "video-720.mp4") {
			t.Fatalf("unsafe source must be ignored, got %q, %v", video, err)
		}
	}
	manifest := strings.ReplaceAll(sourceURLDASHVideoFixture,
		"https://cdn.pstatic.net/audio.m4a?_lsu_sa_=signed", "https://untrusted.example/audio.m4a")
	if _, _, _, err := selectSourceURLDASHVideo([]byte(manifest), "bv*+ba/b"); err == nil {
		t.Fatal("video fallback must require a valid audio source")
	}
	if _, _, _, err := selectSourceURLDASHVideo([]byte(sourceURLDASHVideoFixture),
		"bv*[height<=240]+ba/b[height<=240]"); err == nil {
		t.Fatal("unsupported resolution must fail")
	}
}

func TestBuildSourceURLDASHVideoFFmpegCommandUsesLocalInputs(t *testing.T) {
	status := ToolchainStatus{}
	status.FFmpeg.Available = true
	status.FFmpeg.Path = "ffmpeg.exe"
	dir := t.TempDir()
	videoPath := filepath.Join(dir, "video.mp4")
	audioPath := filepath.Join(dir, "audio.m4a")
	outPath := filepath.Join(dir, "result.mp4")
	spec, err := buildSourceURLDASHVideoFFmpegCommand(status,
		DownloadRequest{DownloadType: DownloadTypeVideo, OutputFormat: "mp4"},
		videoPath, audioPath, outPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{
		{"-i", videoPath}, {"-i", audioPath},
		{"-map", "0:v:0"}, {"-map", "1:a:0"},
		{"-c", "copy"}, {"-movflags", "+faststart"}, {"-f", "mp4"},
	} {
		if !hasArgumentPair(spec.Args, pair[0], pair[1]) {
			t.Errorf("missing FFmpeg option: %#v", pair)
		}
	}
	if strings.Contains(strings.Join(spec.Args, " "), "_lsu_sa_=") {
		t.Fatal("signed URLs must never appear in FFmpeg arguments")
	}
	if spec.Args[len(spec.Args)-1] != outPath {
		t.Fatalf("final output path is not last: %#v", spec.Args)
	}
	if _, err := buildSourceURLDASHVideoFFmpegCommand(status,
		DownloadRequest{DownloadType: DownloadTypeAudio}, videoPath, audioPath, outPath); err == nil {
		t.Fatal("MP3 job must not enter video fallback")
	}
}
