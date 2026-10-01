package downloader

import (
	"path/filepath"
	"testing"
)

func TestStartDownloadRequestValidate(t *testing.T) {
	request := StartDownloadRequest{
		VideoNo:    12345,
		VideoTitle: "테스트 VOD",
		URL:        "https://chzzk.naver.com/video/12345",
		OutputDir:  t.TempDir(),
	}
	if err := request.Validate(); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}

	command := request.DownloadRequest()
	if command.URL != request.URL || command.OutputDir != request.OutputDir {
		t.Fatalf("unexpected download request: %#v", command)
	}
}

func TestStartDownloadRequestRejectsMissingVideo(t *testing.T) {
	request := StartDownloadRequest{
		URL:       "https://chzzk.naver.com/video/12345",
		OutputDir: t.TempDir(),
	}
	if err := request.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestDefaultOutputDir(t *testing.T) {
	path, err := DefaultOutputDir()
	if err != nil {
		t.Skipf("home directory unavailable: %v", err)
	}
	if filepath.Base(path) != "CHZZK Video Downloader" {
		t.Fatalf("unexpected default path: %s", path)
	}
}
