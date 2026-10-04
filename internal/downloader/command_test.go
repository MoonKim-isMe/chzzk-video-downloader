package downloader

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func readyToolchain(dir string) ToolchainStatus {
	return ToolchainStatus{
		YTDLP:         ToolStatus{Name: "yt-dlp", Path: filepath.Join(dir, "yt-dlp.exe"), Found: true, Available: true},
		FFmpeg:        ToolStatus{Name: "ffmpeg", Path: filepath.Join(dir, "ffmpeg.exe"), Found: true, Available: true},
		FFprobe:       ToolStatus{Name: "ffprobe", Path: filepath.Join(dir, "ffprobe.exe"), Found: true, Available: true},
		DownloadReady: true,
		MergeReady:    true,
	}
}

func TestBuildDownloadCommand(t *testing.T) {
	dir := t.TempDir()
	toolchain := readyToolchain(dir)
	spec, err := BuildDownloadCommand(toolchain, DownloadRequest{
		URL:       "https://chzzk.naver.com/video/12345?foo=bar#fragment",
		OutputDir: filepath.Join(dir, "downloads"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if spec.Path != toolchain.YTDLP.Path {
		t.Fatalf("unexpected path: %s", spec.Path)
	}
	if !hasArgumentPair(spec.Args, "--concurrent-fragments", "2") {
		t.Fatalf("default acceleration missing: %#v", spec.Args)
	}
	if !hasArgumentPair(spec.Args, "--encoding", ytDLPOutputEncoding) {
		t.Fatalf("yt-dlp UTF-8 output encoding missing: %#v", spec.Args)
	}
	if !slices.Contains(spec.Env, "PYTHONIOENCODING="+ytDLPOutputEncoding) {
		t.Fatalf("yt-dlp UTF-8 environment missing: %#v", spec.Env)
	}
	if slices.Contains(spec.Args, "--limit-rate") {
		t.Fatalf("unlimited download must omit --limit-rate: %#v", spec.Args)
	}
	expectedTempDir, err := nativeTemporaryDownloadDir(filepath.Join(dir, "downloads"), 12345)
	if err != nil {
		t.Fatal(err)
	}

	for _, expected := range []string{
		"--ignore-config",
		"--no-simulate",
		"--progress",
		"--newline",
		"--continue",
		"--no-keep-fragments",
		"--progress-template",
		"download:" + progressTemplate,
		"--print",
		"after_move:" + finalPathPrefix + "%(filepath)s",
		"--format",
		DefaultFormatSelector,
		"--paths",
		filepath.Join(dir, "downloads"),
		"temp:" + expectedTempDir,
		"--ffmpeg-location",
		dir,
		"https://chzzk.naver.com/video/12345",
	} {
		if !slices.Contains(spec.Args, expected) {
			t.Fatalf("missing argument %q in %#v", expected, spec.Args)
		}
	}
}

func TestBuildDownloadCommandAppliesFormatSelectorAndOutputContainer(t *testing.T) {
	spec, err := BuildDownloadCommand(readyToolchain(t.TempDir()), DownloadRequest{
		URL:                 "https://chzzk.naver.com/video/12345",
		OutputDir:           t.TempDir(),
		FormatSelector:      "bv*[height<=1080]+ba/b[height<=1080]",
		OutputFormat:            "MKV",
		ConcurrentFragments:     8,
		RateLimitBytesPerSecond: 12_500_000,
	})
	if err != nil {
		t.Fatal(err)
	}

	expectedPairs := [][2]string{
		{"--format", "bv*[height<=1080]+ba/b[height<=1080]"},
		{"--merge-output-format", "mkv"},
		{"--remux-video", "mkv"},
		{"--concurrent-fragments", "8"},
		{"--limit-rate", "12500000"},
	}
	for _, pair := range expectedPairs {
		found := false
		for index := 0; index+1 < len(spec.Args); index++ {
			if spec.Args[index] == pair[0] && spec.Args[index+1] == pair[1] {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing argument pair %#v in %#v", pair, spec.Args)
		}
	}
}

func TestBuildDownloadCommandAppliesBrowserAuthentication(t *testing.T) {
	spec, err := BuildDownloadCommand(readyToolchain(t.TempDir()), DownloadRequest{
		URL:       "https://chzzk.naver.com/video/12345",
		OutputDir: t.TempDir(),
		Authentication: AuthenticationOptions{
			Mode: "browser", Browser: "whale", BrowserProfile: "Profile 1",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !hasArgumentPair(spec.Args, "--cookies-from-browser", "whale:Profile 1") {
		t.Fatalf("browser authentication argument missing: %#v", spec.Args)
	}
}

func TestBuildDownloadCommandAppliesCookiesFileAuthentication(t *testing.T) {
	cookiesFile := filepath.Join(t.TempDir(), "cookies.txt")
	spec, err := BuildDownloadCommand(readyToolchain(t.TempDir()), DownloadRequest{
		URL:       "https://chzzk.naver.com/video/12345",
		OutputDir: t.TempDir(),
		Authentication: AuthenticationOptions{
			Mode: "cookies_file", CookiesFilePath: cookiesFile,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !hasArgumentPair(spec.Args, "--cookies", cookiesFile) {
		t.Fatalf("cookies file authentication argument missing: %#v", spec.Args)
	}
}

func TestBuildDownloadCommandRejectsUnsupportedAuthenticationBrowser(t *testing.T) {
	_, err := BuildDownloadCommand(readyToolchain(t.TempDir()), DownloadRequest{
		URL:       "https://chzzk.naver.com/video/12345",
		OutputDir: t.TempDir(),
		Authentication: AuthenticationOptions{Mode: "browser", Browser: "unknown"},
	})
	if err == nil {
		t.Fatal("expected authentication browser validation error")
	}
}

func TestBuildDownloadCommandRejectsUnsupportedConcurrentFragments(t *testing.T) {
	_, err := BuildDownloadCommand(readyToolchain(t.TempDir()), DownloadRequest{
		URL:                 "https://chzzk.naver.com/video/12345",
		OutputDir:           t.TempDir(),
		ConcurrentFragments: 3,
	})
	if err == nil {
		t.Fatal("expected concurrent fragments validation error")
	}
}

func TestBuildDownloadCommandRejectsNegativeRateLimit(t *testing.T) {
	_, err := BuildDownloadCommand(readyToolchain(t.TempDir()), DownloadRequest{
		URL:                     "https://chzzk.naver.com/video/12345",
		OutputDir:               t.TempDir(),
		RateLimitBytesPerSecond: -1,
	})
	if err == nil {
		t.Fatal("expected rate limit validation error")
	}
}

func TestBuildDownloadCommandRejectsUnsupportedOutputFormat(t *testing.T) {
	_, err := BuildDownloadCommand(readyToolchain(t.TempDir()), DownloadRequest{
		URL:          "https://chzzk.naver.com/video/12345",
		OutputDir:    t.TempDir(),
		OutputFormat: "avi",
	})
	if err == nil {
		t.Fatal("expected output format validation error")
	}
}

func TestBuildDownloadCommandRejectsNonChzzkURL(t *testing.T) {
	_, err := BuildDownloadCommand(readyToolchain(t.TempDir()), DownloadRequest{
		URL:       "https://example.com/video/12345",
		OutputDir: t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected URL validation error")
	}
}

func TestBuildDownloadCommandRequiresMergeTools(t *testing.T) {
	toolchain := readyToolchain(t.TempDir())
	toolchain.MergeReady = false
	_, err := BuildDownloadCommand(toolchain, DownloadRequest{
		URL:       "https://chzzk.naver.com/video/12345",
		OutputDir: t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected merge tools error")
	}
}

func TestBuildDownloadCommandRejectsSplitToolDirectories(t *testing.T) {
	toolchain := readyToolchain(t.TempDir())
	toolchain.FFprobe.Path = filepath.Join(t.TempDir(), "ffprobe.exe")
	_, err := BuildDownloadCommand(toolchain, DownloadRequest{
		URL:       "https://chzzk.naver.com/video/12345",
		OutputDir: t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected ffmpeg location error")
	}
}


func TestBuildDownloadCommandUsesResumeInsideVideoTempDirectory(t *testing.T) {
	spec, err := BuildDownloadCommand(readyToolchain(t.TempDir()), DownloadRequest{
		URL:       "https://chzzk.naver.com/video/12345",
		OutputDir: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Contains(spec.Args, "--continue") {
		t.Fatalf("partial resume flag missing: %#v", spec.Args)
	}
	if slices.Contains(spec.Args, "--no-continue") {
		t.Fatalf("partial resume must remain enabled inside the isolated VOD temp directory: %#v", spec.Args)
	}
	if !slices.Contains(spec.Args, "--no-keep-fragments") {
		t.Fatalf("fragment cleanup flag missing: %#v", spec.Args)
	}
}

func TestBuildDownloadCommandUsesFFmpegForHLSFallback(t *testing.T) {
	toolDir := t.TempDir()
	spec, err := buildDownloadCommand(readyToolchain(toolDir), DownloadRequest{
		URL:                 "https://chzzk.naver.com/video/12345",
		OutputDir:           t.TempDir(),
		ConcurrentFragments: 8,
	}, true)
	if err != nil {
		t.Fatal(err)
	}

	expectedPairs := [][2]string{
		{"--downloader", "m3u8:ffmpeg"},
		{"--downloader-args", "ffmpeg:-nostdin"},
		{"--concurrent-fragments", "8"},
	}
	for _, pair := range expectedPairs {
		if !hasArgumentPair(spec.Args, pair[0], pair[1]) {
			t.Fatalf("ffmpeg HLS fallback argument pair missing: %#v in %#v", pair, spec.Args)
		}
	}
	for _, flag := range []string{"--verbose", "--no-quiet"} {
		if !slices.Contains(spec.Args, flag) {
			t.Fatalf("fallback diagnostic flag %q missing: %#v", flag, spec.Args)
		}
	}

	pathInjected := false
	for _, entry := range spec.Env {
		if strings.HasPrefix(strings.ToUpper(entry), "PATH=") &&
			strings.Contains(strings.ToLower(entry), strings.ToLower(toolDir)) {
			pathInjected = true
			break
		}
	}
	if !pathInjected {
		t.Fatalf("ffmpeg tool directory was not injected into PATH: %#v", spec.Env)
	}

	fallbackTemp := commandTempPath(spec.Args)
	if fallbackTemp == "" {
		t.Fatalf("fallback temp path missing: %#v", spec.Args)
	}
	if !hasArgumentPair(spec.Args, "--paths", fallbackTemp) {
		t.Fatalf("fallback home path must be staged with temp path: %#v", spec.Args)
	}
}

func commandTempPath(args []string) string {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == "--paths" && strings.HasPrefix(args[index+1], "temp:") {
			return strings.TrimPrefix(args[index+1], "temp:")
		}
	}
	return ""
}
