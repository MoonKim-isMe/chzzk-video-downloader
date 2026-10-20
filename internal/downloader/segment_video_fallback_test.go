package downloader

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"testing"
)

func TestVideoSegmentYTDLPFailureTriggersSignedSourceURLFallback(t *testing.T) {
	processFailure := &ProcessError{Program: "yt-dlp.exe", ExitCode: 1, StderrTail: []string{"ERROR: ffmpeg exited with code 1"}}
	segment := &DownloadSegment{Title: "첫 구간", Position: 1, StartSeconds: 30}
	video := DownloadRequest{DownloadType: DownloadTypeVideo, Segment: segment}
	audio := DownloadRequest{DownloadType: DownloadTypeAudio, Segment: segment}
	full := DownloadRequest{DownloadType: DownloadTypeVideo}
	for _, tc := range []struct {
		name string
		request DownloadRequest
		err error
		want bool
	}{
		{"video segment subprocess error", video, processFailure, true},
		{"audio segment process error unchanged", audio, processFailure, false},
		{"whole video process error unchanged", full, processFailure, false},
		{"signed video extractor error", video, classifyDownloadFailure(&ProcessError{Program:"yt-dlp.exe", ExitCode:1, StderrTail: []string{"KeyError('sourceURL')"}}), true},
		{"discontinuity error", video, &DownloadFailure{Kind: DownloadFailureHLSInitializationFragmentOrder, Cause: processFailure}, true},
		{"yt-dlp exited without final path", video, fmt.Errorf("다운로드는 완료되었지만 최종 파일 경로를 확인할 수 없습니다"), true},
		{"login required does not retry", video, NewAuthenticationRequiredFailure(processFailure), false},
		{"partial data conflict does not retry", video, &DownloadFailure{Kind: DownloadFailurePartialDataConflict, Cause: processFailure}, false},
		{"cancelled job does not retry", video, context.Canceled, false},
		{"FFmpeg error is not a yt-dlp subprocess error", video, &ProcessError{Program:"ffmpeg.exe", ExitCode:1}, false},
		{"unrelated filesystem error does not retry", video, fmt.Errorf("permission denied"), false},
		{"successful download does not retry", video, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldFallbackVideoSegment(tc.request, tc.err); got != tc.want {
				t.Fatalf("shouldFallbackVideoSegment() = %t, expected %t", got, tc.want)
			}
		})
	}
}

func TestVideoSegmentPrefersMuxedHLSFormatWithoutChangingAudioOrExplicitQuality(t *testing.T) {
	status := readyToolchain(t.TempDir())
	request := DownloadRequest{
		URL: "https://chzzk.naver.com/video/123456",
		OutputDir: t.TempDir(),
		DownloadType: DownloadTypeVideo,
		Segment: &DownloadSegment{Title: "clip", Position: 1, StartSeconds: 20},
	}
	spec, err := BuildDownloadCommand(status, request)
	if err != nil { t.Fatal(err) }
	if !hasArgumentPair(spec.Args, "--format", "b[vcodec!=none][acodec!=none]/"+DefaultFormatSelector) {
		t.Fatalf("video segment must prefer a muxed HLS format: %v", spec.Args)
	}
	if !slices.Contains(spec.Args, "--force-keyframes-at-cuts") {
		t.Fatal("normal video section should retain accurate keyframe cutting")
	}
	request.Segment = nil
	normal, err := BuildDownloadCommand(status, request)
	if err != nil { t.Fatal(err) }
	if !hasArgumentPair(normal.Args, "--format", DefaultFormatSelector) {
		t.Fatal("whole video format selector changed")
	}
	request.Segment = &DownloadSegment{Title: "clip", Position: 1, StartSeconds: 20}
	request.FormatSelector = "bv*[height<=720]+ba/b[height<=720]"
	quality, err := BuildDownloadCommand(status, request)
	if err != nil { t.Fatal(err) }
	if !hasArgumentPair(quality.Args, "--format", request.FormatSelector) {
		t.Fatal("explicit resolution choice was overridden")
	}
	request.DownloadType = DownloadTypeAudio
	audio, err := BuildDownloadCommand(status, request)
	if err != nil { t.Fatal(err) }
	if !hasArgumentPair(audio.Args, "--format", "bestaudio/best") {
		t.Fatal("MP3 segment extraction path was changed")
	}
}

func TestSourceURLVideoClipSupportsWebMWithEncoding(t *testing.T) {
	request := DownloadRequest{
		DownloadType: DownloadTypeVideo,
		OutputFormat: "webm",
		Segment: &DownloadSegment{Title: "clip", Position: 1, StartSeconds: 15},
	}
	format, err := sourceURLFallbackOutputFormat(request)
	if err != nil || format != "webm" {
		t.Fatalf("video clip output should support WebM: %q %v", format, err)
	}
	status := readyToolchain(t.TempDir())
	playlist := filepath.Join(t.TempDir(), "playlist.m3u8")
	spec, err := buildSourceURLFFmpegCommand(status, request, sourceURLHLSStream{PlaylistPath: playlist}, filepath.Join(t.TempDir(), "clip.webm"))
	if err != nil { t.Fatal(err) }
	if !hasArgumentPair(spec.Args, "-c", "copy") {
		t.Fatalf("WebM clip must first attempt original HLS stream copy: %v", spec.Args)
	}
	retry, err := buildSourceURLFFmpegCommand(status, request, sourceURLHLSStream{PlaylistPath: playlist}, filepath.Join(t.TempDir(), "clip.webm"), "libvpx-vp9")
	if err != nil { t.Fatal(err) }
	if !hasArgumentPair(retry.Args, "-c:v", "libvpx-vp9") || !hasArgumentPair(retry.Args, "-c:a", "libopus") {
		t.Fatalf("WebM clip fallback must use video + audio encoding: %v", retry.Args)
	}
	request.Segment = nil
	if _, err := sourceURLFallbackOutputFormat(request); err == nil {
		t.Fatal("whole VOD WebM lossless fallback must remain unsupported")
	}
}
