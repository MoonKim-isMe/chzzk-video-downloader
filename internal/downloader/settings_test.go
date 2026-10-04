package downloader

import (
	"math"
	"path/filepath"
	"testing"

	appsettings "github.com/MoonKim-isMe/chzzk-video-downloader/internal/settings"
)

func TestConcurrentFragmentsForAcceleration(t *testing.T) {
	cases := map[appsettings.DownloadAcceleration]int{
		appsettings.DownloadAccelerationStable:   1,
		appsettings.DownloadAccelerationStandard: 2,
		appsettings.DownloadAccelerationFast:     4,
		appsettings.DownloadAccelerationUltra:    8,
	}

	for acceleration, expected := range cases {
		actual, err := ConcurrentFragmentsForAcceleration(acceleration)
		if err != nil {
			t.Fatalf("%s: %v", acceleration, err)
		}
		if actual != expected {
			t.Fatalf("%s: got %d want %d", acceleration, actual, expected)
		}
	}
}

func TestRateLimitBytesPerSecond(t *testing.T) {
	cases := map[float64]int64{
		0:    0,
		0.01: 10_000,
		1.25: 1_250_000,
		12.5: 12_500_000,
	}

	for rateLimitMBps, expected := range cases {
		actual, err := RateLimitBytesPerSecond(rateLimitMBps)
		if err != nil {
			t.Fatalf("%v MB/s: %v", rateLimitMBps, err)
		}
		if actual != expected {
			t.Fatalf("%v MB/s: got %d want %d", rateLimitMBps, actual, expected)
		}
	}
}

func TestRateLimitBytesPerSecondRejectsInvalidValue(t *testing.T) {
	for _, value := range []float64{-1, math.NaN(), math.Inf(1)} {
		if _, err := RateLimitBytesPerSecond(value); err == nil {
			t.Fatalf("expected invalid rate limit error for %v", value)
		}
	}
}

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
		VideoNo:      12345,
		VideoTitle:   "VOD",
		URL:          "https://chzzk.naver.com/video/12345",
		OutputDir:    "caller-value",
		OutputFormat: "caller-format",
	}, appsettings.AppSettings{
		DownloadDir:            outputDir,
		Resolution:             appsettings.Resolution1080p,
		OutputFormat:           appsettings.OutputFormatMKV,
		DownloadAcceleration:   appsettings.DownloadAccelerationFast,
		DownloadRateLimitMBps:   12.5,
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
	if request.ConcurrentFragments != 4 {
		t.Fatalf("unexpected concurrent fragments: %d", request.ConcurrentFragments)
	}
	if request.RateLimitBytesPerSecond != 12_500_000 {
		t.Fatalf("unexpected rate limit: %d", request.RateLimitBytesPerSecond)
	}
}

func TestApplyAuthenticationSettingsSnapshotsBrowserSession(t *testing.T) {
	request, err := ApplyAuthenticationSettings(StartDownloadRequest{}, appsettings.AuthenticationSettings{
		Enabled:        true,
		Mode:           appsettings.AuthenticationModeBrowser,
		Browser:        appsettings.AuthenticationBrowserWhale,
		BrowserProfile: "Profile 1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if request.Authentication.Mode != "browser" ||
		request.Authentication.Browser != "whale" ||
		request.Authentication.BrowserProfile != "Profile 1" {
		t.Fatalf("unexpected authentication snapshot: %#v", request.Authentication)
	}
}

func TestApplyAuthenticationSettingsDisabledClearsAuthentication(t *testing.T) {
	request, err := ApplyAuthenticationSettings(StartDownloadRequest{
		Authentication: AuthenticationOptions{Mode: "browser", Browser: "chrome"},
	}, appsettings.AuthenticationDefaults())
	if err != nil {
		t.Fatal(err)
	}
	if request.Authentication != (AuthenticationOptions{}) {
		t.Fatalf("disabled authentication should clear snapshot: %#v", request.Authentication)
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
					DownloadAcceleration:   appsettings.DownloadAccelerationStandard,
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
				if !hasArgumentPair(spec.Args, "--concurrent-fragments", "2") {
					t.Fatalf("missing standard acceleration in %#v", spec.Args)
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
