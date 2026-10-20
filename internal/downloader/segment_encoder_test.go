package downloader

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

var minimalFFmpegEncoders = strings.Join([]string{
	"Encoders:",
	" V..... = Video",
	" A..... = Audio",
	" V.S... mpeg4                MPEG-4 part 2",
	" A....D aac                  AAC (Advanced Audio Coding)",
}, "\n")

var fullFFmpegEncoders = strings.Join([]string{
	"Encoders:",
	" V....D libx264              libx264 H.264 / AVC / MPEG-4 AVC (codec h264)",
	" V.S... mpeg4                MPEG-4 part 2",
	" V....D libvpx-vp9           libvpx VP9 (codec vp9)",
	" V....D libvpx               libvpx VP8 (codec vp8)",
}, "\n")

func TestSelectVideoEncoderFromInstalledFFmpegList(t *testing.T) {
	tests := []struct {
		name, format, listing, want string
		wantError bool
	}{
		{"prefers h264 when installed", "mp4", fullFFmpegEncoders, "libx264", false},
		{"uses native mpeg4 when x264 is missing", "mp4", minimalFFmpegEncoders, "mpeg4", false},
		{"mkv also supports native mpeg4", "mkv", minimalFFmpegEncoders, "mpeg4", false},
		{"webm uses vp9 when present", "webm", fullFFmpegEncoders, "libvpx-vp9", false},
		{"webm falls back to vp8", "webm", " V..... libvpx   libvpx VP8\n", "libvpx", false},
		{"webm without codecs reports missing encoder", "webm", minimalFFmpegEncoders, "", true},
		{"missing all video encoders reports failure", "mp4", " V..... foo\n", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := chooseSegmentVideoEncoder(tt.format, tt.listing)
			if (err != nil) != tt.wantError || got != tt.want {
				t.Fatalf("chooseSegmentVideoEncoder(%s) = (%q, %v), want %q error=%v", tt.format, got, err, tt.want, tt.wantError)
			}
		})
	}
}

func TestFFmpegEncodeOptionsDoNotRequireMissingX264(t *testing.T) {
	encoder, err := chooseSegmentVideoEncoder("mp4", minimalFFmpegEncoders)
	if err != nil { t.Fatal(err) }
	args, err := segmentVideoEncodingArgs("mp4", encoder)
	if err != nil { t.Fatal(err) }
	if !hasArgumentPair(args, "-c:v", "mpeg4") || !hasArgumentPair(args, "-q:v", "3") ||
		!hasArgumentPair(args, "-pix_fmt", "yuv420p") || !hasArgumentPair(args, "-c:a", "aac") {
		t.Fatalf("native MP4 codec not configured correctly: %v", args)
	}
	if slices.Contains(args, "libx264") || slices.Contains(args, "-crf") ||
		slices.Contains(args, "-preset") {
		t.Fatalf("native mpeg4 cannot accept x264-specific arguments: %v", args)
	}
	args, err = segmentVideoEncodingArgs("mp4", "libx264")
	if err != nil { t.Fatal(err) }
	if !hasArgumentPair(args, "-c:v", "libx264") || !hasArgumentPair(args, "-crf", "20") {
		t.Fatal("x264 settings not preserved when encoder exists")
	}
}

func TestSignedHLSFFmpegCommandUsesProbedNativeEncoder(t *testing.T) {
	status := readyToolchain(t.TempDir())
	end := int64(131)
	request := DownloadRequest{
		DownloadType: DownloadTypeVideo,
		OutputFormat: "mp4",
		Segment: &DownloadSegment{Title: "구간 1", Position: 1, StartSeconds: 0, EndSeconds: &end},
	}
	stream := sourceURLHLSStream{PlaylistPath: filepath.Join(t.TempDir(), "playlist.m3u8")}
	encoder, err := chooseSegmentVideoEncoder("mp4", minimalFFmpegEncoders)
	if err != nil { t.Fatal(err) }
	spec, err := buildSourceURLFFmpegCommand(status, request, stream, "01. 구간 1.mp4", encoder)
	if err != nil { t.Fatal(err) }
	if !hasArgumentPair(spec.Args, "-c:v", "mpeg4") || !hasArgumentPair(spec.Args, "-t", "132") {
		t.Fatalf("expected native video encoding and selected segment duration: %v", spec.Args)
	}
	if slices.Contains(spec.Args, "libx264") || !hasArgumentPair(spec.Args, "-movflags", "+faststart") {
		t.Fatalf("video clip should remain MP4 without libx264 dependency: %v", spec.Args)
	}
	request.DownloadType = DownloadTypeAudio
	audio, err := buildSourceURLFFmpegCommand(status, request, stream, "01. 구간 1.mp3")
	if err != nil { t.Fatal(err) }
	if !hasArgumentPair(audio.Args, "-codec:a", "libmp3lame") {
		t.Fatal("MP3 fallback should remain unchanged")
	}
}

func TestFFmpegEncoderProbeRejectsMissingExecutable(t *testing.T) {
	if _, err := probeSegmentVideoEncoder(context.Background(), "", "mp4"); err == nil ||
		!strings.Contains(err.Error(), "경로") {
		t.Fatal("empty FFmpeg executable path must fail explicitly")
	}
}

func TestNativeMPEG4EncodeArgsDoNotChangeContainerName(t *testing.T) {
	args, err := segmentVideoEncodingArgs("mkv", "mpeg4")
	if err != nil { t.Fatal(err) }
	if !hasArgumentPair(args, "-c:v", "mpeg4") {
		t.Fatal("MKV must support native mpeg4 fallback")
	}
	if _, err := segmentVideoEncodingArgs("webm", "mpeg4"); err == nil {
		t.Fatal("mpeg4 is incompatible with WebM")
	}
}


func TestSignedHLSSegmentAttemptsCopyBeforeOptionalReencoding(t *testing.T) {
	req := DownloadRequest{
		DownloadType: DownloadTypeVideo, OutputFormat: "mp4",
		Segment: &DownloadSegment{Title: "intro", Position: 1, StartSeconds: 30},
	}
	status := readyToolchain(t.TempDir())
	stream := sourceURLHLSStream{PlaylistPath: filepath.Join(t.TempDir(), "manifest.m3u8")}
	path := filepath.Join(t.TempDir(), "intro.mp4")

	copySpec, err := buildSourceURLFFmpegCommand(status, req, stream, path)
	if err != nil { t.Fatal(err) }
	if !hasArgumentPair(copySpec.Args, "-c", "copy") || slices.Contains(copySpec.Args, "libx264") {
		t.Fatalf("initial video clip must not require libx264: %v", copySpec.Args)
	}
	if !hasArgumentPair(copySpec.Args, "-ss", "00:00:30") {
		t.Fatalf("initial copy must preserve section start: %v", copySpec.Args)
	}
	retrySpec, err := buildSourceURLFFmpegCommand(status, req, stream, path, "mpeg4")
	if err != nil { t.Fatal(err) }
	if !hasArgumentPair(retrySpec.Args, "-c:v", "mpeg4") || !hasArgumentPair(retrySpec.Args, "-q:v", "3") ||
		!hasArgumentPair(retrySpec.Args, "-ss", "00:00:30") {
		t.Fatalf("reencode fallback must preserve clip range: %v", retrySpec.Args)
	}
	if slices.Contains(retrySpec.Args, "libx264") || slices.Contains(retrySpec.Args, "copy") {
		t.Fatalf("the fallback must not require absent encoder or stream copy: %v", retrySpec.Args)
	}

	req.Segment = nil
	wholeSpec, err := buildSourceURLFFmpegCommand(status, req, stream, path)
	if err != nil { t.Fatal(err) }
	if !hasArgumentPair(wholeSpec.Args, "-c", "copy") {
		t.Fatal("whole video fallback should remain copy-only")
	}
	req.DownloadType = DownloadTypeAudio
	audioSpec, err := buildSourceURLFFmpegCommand(status, req, stream, filepath.Join(t.TempDir(), "audio.mp3"))
	if err != nil { t.Fatal(err) }
	if !hasArgumentPair(audioSpec.Args, "-codec:a", "libmp3lame") {
		t.Fatal("audio fallback must remain separate from video codec changes")
	}
}
