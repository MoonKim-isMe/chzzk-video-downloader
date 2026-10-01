package downloader

import (
	"path/filepath"
	"testing"

	appsettings "github.com/MoonKim-isMe/chzzk-video-downloader/internal/settings"
)

func TestFormatSelectorForResolution(t *testing.T) {
	cases := map[appsettings.Resolution]string{
		appsettings.ResolutionBest:  DefaultFormatSelector,
		appsettings.Resolution2160p: "bv*[height<=2160]+ba/b[height<=2160]",
		appsettings.Resolution1440p: "bv*[height<=1440]+ba/b[height<=1440]",
		appsettings.Resolution1080p: "bv*[height<=1080]+ba/b[height<=1080]",
		appsettings.Resolution720p:  "bv*[height<=720]+ba/b[height<=720]",
	}

	for resolution, expected := range cases {
		actual, err := FormatSelectorForResolution(resolution)
		if err != nil {
			t.Fatalf("%s: %v", resolution, err)
		}
		if actual != expected {
			t.Fatalf("%s: got %q want %q", resolution, actual, expected)
		}
	}
}

func TestApplySettingsSnapshotsDownloadOptions(t *testing.T) {
	outputDir := filepath.Join(t.TempDir(), "configured")
	request, err := ApplySettings(StartDownloadRequest{
		VideoNo:     12345,
		VideoTitle:  "VOD",
		URL:         "https://chzzk.naver.com/video/12345",
		OutputDir:   "caller-value",
		OutputFormat: "caller-format",
	}, appsettings.AppSettings{
		DownloadDir:            outputDir,
		Resolution:             appsettings.Resolution1080p,
		OutputFormat:           appsettings.OutputFormatMKV,
		MaxConcurrentDownloads: 3,
	})
	if err != nil {
		t.Fatal(err)
	}

	if request.OutputDir != outputDir {
		t.Fatalf("unexpected output dir: %s", request.OutputDir)
	}
	if request.FormatSelector != "bv*[height<=1080]+ba/b[height<=1080]" {
		t.Fatalf("unexpected format selector: %s", request.FormatSelector)
	}
	if request.OutputFormat != "mkv" {
		t.Fatalf("unexpected output format: %s", request.OutputFormat)
	}
}

func TestApplySettingsRejectsInvalidSettings(t *testing.T) {
	_, err := ApplySettings(StartDownloadRequest{}, appsettings.AppSettings{
		DownloadDir:            t.TempDir(),
		Resolution:             "480p",
		OutputFormat:           appsettings.OutputFormatMP4,
		MaxConcurrentDownloads: 1,
	})
	if err == nil {
		t.Fatal("expected settings validation error")
	}
}
