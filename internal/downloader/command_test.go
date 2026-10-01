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
	for _, expected := range []string{
		"--ignore-config",
		"--no-simulate",
		"--progress",
		"--newline",
		"--progress-template",
		"download:" + progressTemplate,
		"--print",
		"after_move:" + finalPathPrefix + "%(filepath)s",
		"--format",
		DefaultFormatSelector,
		"--ffmpeg-location",
		dir,
		"https://chzzk.naver.com/video/12345",
	} {
		if !slices.Contains(spec.Args, expected) {
			t.Fatalf("missing argument %q in %#v", expected, spec.Args)
		}
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
