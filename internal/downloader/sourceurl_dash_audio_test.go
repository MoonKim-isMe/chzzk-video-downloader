package downloader

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestSourceURLDASHAudioSelectedWithoutHLS(t *testing.T) {
	manifest := strings.Replace(sourceURLFixture,
		"https://example.org/track.m4a",
		"https://b01-kr-naver-vod.pstatic.net/track.m4a?_lsu_sa_=signed", 1)
	// The DASH audio AdaptationSet is present even when CHZZK has no
	// muxed video/mp2t HLS representations.
	manifest = strings.Replace(manifest, `mimeType="video/mp2t"`, `mimeType="video/mp4"`, 1)
	address, err := selectSourceURLDASHAudio([]byte(manifest))
	if err != nil || !strings.Contains(address, "track.m4a?_lsu_sa_=") {
		t.Fatalf("DASH audio should be selected: %v", err)
	}
	if _, _, hlsErr := selectSourceURLHLSForType([]byte(manifest), "bestaudio/best", DownloadTypeAudio); hlsErr == nil {
		t.Fatal("fixture must not contain a selectable HLS stream")
	}
}

func TestSourceURLDASHAudioRejectsOtherOrigins(t *testing.T) {
	for _, address := range []string{
		"https://example.org/track.m4a",
		"http://b01-kr-naver-vod.pstatic.net/track.m4a",
		"https://pstatic.net.evil.org/track.mp4",
		"https://u:p@pstatic.net/track.mp4",
		"https://pstatic.net:8080/track.m4a",
		"https://pstatic.net/video.m3u8",
	} {
		if validSourceURLDASHAudioAddress(address) {
			t.Errorf("unsafe DASH audio URL accepted: %s", address)
		}
	}
	if _, err := selectSourceURLDASHAudio([]byte(sourceURLFixture)); err == nil {
		t.Fatal("must reject untrusted DASH audio URL")
	}
}

func TestBuildSourceURLDASHAudioCommandNoSignedAddress(t *testing.T) {
	toolchain := ToolchainStatus{}
	toolchain.FFmpeg.Available = true
	toolchain.FFmpeg.Path = "ffmpeg.exe"
	input := filepath.Join(t.TempDir(), "audio-input.mp4")
	output := filepath.Join(t.TempDir(), "final.mp3")
	spec, err := buildSourceURLDASHAudioFFmpegCommand(toolchain,
		DownloadRequest{DownloadType: DownloadTypeAudio}, input, output)
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{{"-i", input}, {"-map", "0:a:0"}, {"-codec:a", "libmp3lame"}, {"-q:a", "0"}, {"-f", "mp3"}} {
		if !hasArgumentPair(spec.Args, pair[0], pair[1]) {
			t.Errorf("missing audio converter option: %v", pair)
		}
	}
	if !slices.Contains(spec.Args, "-vn") || spec.Args[len(spec.Args)-1] != output {
		t.Fatalf("unexpected MP3 command: %#v", spec.Args)
	}
	if _, err := buildSourceURLDASHAudioFFmpegCommand(toolchain,
		DownloadRequest{DownloadType: DownloadTypeVideo}, input, output); err == nil {
		t.Fatal("DASH audio conversion must only accept MP3 jobs")
	}
}

type sourceURLDASHRoundTrip func(*http.Request) (*http.Response, error)

func (fn sourceURLDASHRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}

func TestDownloadSourceURLDASHAudioRequiresCompleteHTTP200(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusPartialContent, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			client := &http.Client{Transport: sourceURLDASHRoundTrip(func(r *http.Request) (*http.Response, error) {
				if r.Header.Get("Origin") != "https://chzzk.naver.com" {
					t.Error("missing CHZZK origin")
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("mp4-audio")),
					ContentLength: 9, Header: make(http.Header), Request: r}, nil
			})}
			path := filepath.Join(t.TempDir(), "received.mp4")
			err := downloadSourceURLDASHAudio(context.Background(), client,
				"https://cdn.pstatic.net/recording.m4a?_lsu_sa_=secret", path, nil)
			if status == http.StatusOK {
				if err != nil {
					t.Fatal(err)
				}
				if _, err := os.Stat(path); err != nil {
					t.Fatal(err)
				}
			} else {
				if err == nil {
					t.Fatalf("expected HTTP %d rejection", status)
				}
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("partial output must not exist after failure: %v", err)
				}
			}
		})
	}
}
