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


func TestSettingsBuildDownloadCommandMatrix(t *testing.T) {
	resolutions := []appsettings.Resolution{
		appsettings.ResolutionBest,
		appsettings.Resolution2160p,
		appsettings.Resolution1440p,
		appsettings.Resolution1080p,
		appsettings.Resolution720p,
	}
	formats := []appsettings.OutputFormat{
		appsettings.OutputFormatMP4,
		appsettings.OutputFormatMKV,
		appsettings.OutputFormatWebM,
	}

	for _, resolution := range resolutions {
		for _, outputFormat := range formats {
			t.Run(string(resolution)+"_"+string(outputFormat), func(t *testing.T) {
				outputDir := filepath.Join(t.TempDir(), "matrix")
				request, err := ApplySettings(StartDownloadRequest{
					VideoNo:    12345,
					VideoTitle: "VOD",
					URL:        "https://chzzk.naver.com/video/12345",
				}, appsettings.AppSettings{
					DownloadDir:            outputDir,
					Resolution:             resolution,
					OutputFormat:           outputFormat,
					MaxConcurrentDownloads: 1,
				})
				if err != nil {
					t.Fatal(err)
				}

				spec, err := BuildDownloadCommand(readyToolchain(t.TempDir()), request.DownloadRequest())
				if err != nil {
					t.Fatal(err)
				}

				expectedSelector, err := FormatSelectorForResolution(resolution)
				if err != nil {
					t.Fatal(err)
				}
				if !hasArgumentPair(spec.Args, "--format", expectedSelector) {
					t.Fatalf("missing format selector %q in %#v", expectedSelector, spec.Args)
				}
				if !hasArgumentPair(spec.Args, "--merge-output-format", string(outputFormat)) {
					t.Fatalf("missing merge format %q in %#v", outputFormat, spec.Args)
				}
				if !hasArgumentPair(spec.Args, "--remux-video", string(outputFormat)) {
					t.Fatalf("missing remux format %q in %#v", outputFormat, spec.Args)
				}
				if !hasArgumentPair(spec.Args, "--paths", outputDir) {
					t.Fatalf("missing output path %q in %#v", outputDir, spec.Args)
				}
			})
		}
	}
}

func hasArgumentPair(args []string, key, value string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == key && args[index+1] == value {
			return true
		}
	}
	return false
}
