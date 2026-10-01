package downloader

import (
	"path/filepath"
	"slices"
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
	expectedTempDir, err := TemporaryDownloadDir(filepath.Join(dir, "downloads"), 12345)
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
		URL:            "https://chzzk.naver.com/video/12345",
		OutputDir:      t.TempDir(),
		FormatSelector: "bv*[height<=1080]+ba/b[height<=1080]",
		OutputFormat:   "MKV",
	})
	if err != nil {
		t.Fatal(err)
	}

	expectedPairs := [][2]string{
		{"--format", "bv*[height<=1080]+ba/b[height<=1080]"},
		{"--merge-output-format", "mkv"},
		{"--remux-video", "mkv"},
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


func TestBuildDownloadCommandDoesNotResumeCancelledPartialData(t *testing.T) {
	spec, err := BuildDownloadCommand(readyToolchain(t.TempDir()), DownloadRequest{
		URL:       "https://chzzk.naver.com/video/12345",
		OutputDir: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Contains(spec.Args, "--no-continue") {
		t.Fatalf("retry safety flag missing: %#v", spec.Args)
	}
	if slices.Contains(spec.Args, "--continue") {
		t.Fatalf("partial resume must be disabled after cancellation: %#v", spec.Args)
	}
	if !slices.Contains(spec.Args, "--no-keep-fragments") {
		t.Fatalf("fragment cleanup flag missing: %#v", spec.Args)
	}
}
