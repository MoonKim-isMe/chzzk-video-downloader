package downloader

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestSelectSourceURLHLSForAudioUsesMuxedAACStream(t *testing.T) {
	address, height, err := selectSourceURLHLSForType(
		[]byte(sourceURLFixture), "bestaudio/best", DownloadTypeAudio,
	)
	if err != nil {
		t.Fatal(err)
	}
	// The MPD fixture contains audio/mp4 ranges without an HLS URL,
	// so the fallback must use a signed video/mp2t stream containing AAC.
	if height != 1080 || !strings.Contains(address, "/1080.m3u8?signature=") {
		t.Fatalf("unexpected AAC HLS selection: %d %q", height, address)
	}
	videoAddress, videoHeight, err := selectSourceURLHLS([]byte(sourceURLFixture), "bv*+ba/b")
	if err != nil || videoAddress != address || videoHeight != height {
		t.Fatalf("video selection changed: address=%q height=%d err=%v", videoAddress, videoHeight, err)
	}
}

func TestSelectSourceURLHLSForAudioRejectsUnsupportedSelector(t *testing.T) {
	_, _, err := selectSourceURLHLSForType(
		[]byte(sourceURLFixture), "bv*+ba/b", DownloadTypeAudio,
	)
	if err == nil || !strings.Contains(err.Error(), "오디오 선택식") {
		t.Fatalf("unexpected audio selector validation: %v", err)
	}
}

func TestSelectSourceURLHLSForAudioRequiresAAC(t *testing.T) {
	const noAudioFixture = `<?xml version="1.0"?>
<MPD xmlns:nvod="urn:naver:vod:2020"><Period>
<AdaptationSet mimeType="video/mp2t">
<Representation height="1080" bandwidth="5000000" codecs="avc1.64002a" nvod:m3u="https://cdn.pstatic.net/no-audio.m3u8"/>
</AdaptationSet>
</Period></MPD>`
	_, _, err := selectSourceURLHLSForType(
		[]byte(noAudioFixture), "bestaudio/best", DownloadTypeAudio,
	)
	if err == nil {
		t.Fatal("MP3 fallback must not select a video-only stream")
	}
}

func TestSourceURLFallbackOutputFormatMatchesDownloadType(t *testing.T) {
	tests := []struct {
		request DownloadRequest
		want    string
	}{
		{DownloadRequest{DownloadType: DownloadTypeAudio}, "mp3"},
		{DownloadRequest{DownloadType: DownloadTypeAudio, OutputFormat: "webm"}, "mp3"},
		{DownloadRequest{DownloadType: DownloadTypeVideo}, "mp4"},
		{DownloadRequest{DownloadType: DownloadTypeVideo, OutputFormat: "mkv"}, "mkv"},
		{DownloadRequest{OutputFormat: "mp4"}, "mp4"},
	}
	for _, test := range tests {
		got, err := sourceURLFallbackOutputFormat(test.request)
		if err != nil || got != test.want {
			t.Errorf("output format for %+v = %q, %v; want %q", test.request, got, err, test.want)
		}
	}
	if _, err := sourceURLFallbackOutputFormat(DownloadRequest{DownloadType: DownloadTypeVideo, OutputFormat: "webm"}); err == nil {
		t.Fatal("video fallback must continue rejecting WebM stream-copy")
	}
}

func TestBuildSourceURLFFmpegCommandTranscodesMP3AudioOnly(t *testing.T) {
	status := ToolchainStatus{}
	status.FFmpeg.Available = true
	status.FFmpeg.Path = "ffmpeg.exe"
	stream := sourceURLHLSStream{
		URL: "https://cdn.pstatic.net/playlist.m3u8?_lsu_sa_=secret",
		PlaylistPath: filepath.Join(t.TempDir(), "playlist.m3u8"),
	}
	request := DownloadRequest{DownloadType: DownloadTypeAudio, OutputFormat: "webm"}
	spec, err := buildSourceURLFFmpegCommand(status, request, stream, "audio.mp3")
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{
		{"-map", "0:a:0"},
		{"-codec:a", "libmp3lame"},
		{"-q:a", "0"},
		{"-f", "mp3"},
		{"-progress", "pipe:1"},
		{"-protocol_whitelist", "file,http,https,tcp,tls,crypto"},
		{"-seg_max_retry", "3"},
	} {
		if !hasArgumentPair(spec.Args, pair[0], pair[1]) {
			t.Errorf("missing MP3 FFmpeg option: %#v in %#v", pair, spec.Args)
		}
	}
	for _, flag := range []string{"-n", "-vn"} {
		if !slices.Contains(spec.Args, flag) {
			t.Errorf("missing MP3 flag %s in %#v", flag, spec.Args)
		}
	}
	joinedArgs := strings.Join(spec.Args, " ")
	for _, forbidden := range []string{"0:v:0", "0:a:0?", "-c copy", "+faststart", "matroska", "_lsu_sa_=secret", "-user_agent"} {
		if strings.Contains(joinedArgs, forbidden) {
			t.Errorf("MP3 fallback contains video copy, remote secret or invalid option: %q in %#v", forbidden, spec.Args)
		}
	}
	if spec.Args[len(spec.Args)-1] != "audio.mp3" {
		t.Fatalf("unexpected MP3 output: %#v", spec.Args)
	}
	if _, err := buildSourceURLFFmpegCommand(status, request, stream, "audio.mp4"); err == nil {
		t.Fatal("MP3 fallback must reject a video output extension")
	}
}

func TestSourceURLAudioCacheIsIsolatedAndResumable(t *testing.T) {
	root := t.TempDir()
	manifest1 := filepath.Join(root, "manifest-a")
	manifest2 := filepath.Join(root, "manifest-b")
	writeSourceURLCacheFixture(t, manifest1, "signature-one", "4.0")
	writeSourceURLCacheFixture(t, manifest2, "signature-two", "4.0")

	audio1, err := sourceURLSegmentCacheDirForType(root, 15651133, 1080, manifest1, DownloadTypeAudio)
	if err != nil {
		t.Fatal(err)
	}
	audio2, err := sourceURLSegmentCacheDirForType(root, 15651133, 1080, manifest2, DownloadTypeAudio)
	if err != nil {
		t.Fatal(err)
	}
	video, err := sourceURLSegmentCacheDirForType(root, 15651133, 1080, manifest1, DownloadTypeVideo)
	if err != nil {
		t.Fatal(err)
	}
	legacyVideo, err := sourceURLSegmentCacheDir(root, 15651133, 1080, manifest1)
	if err != nil {
		t.Fatal(err)
	}
	if audio1 != audio2 || audio1 == video || video != legacyVideo {
		t.Fatalf("audio resume/cache isolation failed: audio1=%q audio2=%q video=%q legacy=%q", audio1, audio2, video, legacyVideo)
	}
	if filepath.Base(filepath.Dir(audio1)) != "audio" {
		t.Fatalf("MP3 must use its own cache namespace: %q", audio1)
	}
	if strings.Contains(audio1, "signature-one") || strings.Contains(audio1, "signature-two") {
		t.Fatal("signed URL leaked into MP3 cache path")
	}
}
