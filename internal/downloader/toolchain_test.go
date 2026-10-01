package downloader

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestResolverPrefersConfiguredDirectory(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"yt-dlp.exe", "ffmpeg.exe", "ffprobe.exe"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("test"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	resolver := &Resolver{
		searchDirs: []searchDir{{path: dir, source: "test"}},
		lookPath: func(string) (string, error) {
			return "", errors.New("not found")
		},
		probe: func(_ context.Context, path, arg string) (string, error) {
			return filepath.Base(path) + " " + arg, nil
		},
	}

	status := resolver.Resolve(context.Background())
	if !status.DownloadReady || !status.MergeReady {
		t.Fatalf("unexpected status: %#v", status)
	}
	if status.YTDLP.Source != "test" || filepath.Base(status.YTDLP.Path) != "yt-dlp.exe" {
		t.Fatalf("unexpected yt-dlp resolution: %#v", status.YTDLP)
	}
}

func TestResolverReportsUnusableBinary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "yt-dlp")
	if err := os.WriteFile(path, []byte("test"), 0o755); err != nil {
		t.Fatal(err)
	}
	resolver := &Resolver{
		searchDirs: []searchDir{{path: dir, source: "test"}},
		lookPath:   func(string) (string, error) { return "", errors.New("not found") },
		probe: func(_ context.Context, path, arg string) (string, error) {
			return "", errors.New("broken")
		},
	}
	status := resolver.Resolve(context.Background())
	if !status.YTDLP.Found || status.YTDLP.Available || status.DownloadReady {
		t.Fatalf("unexpected status: %#v", status.YTDLP)
	}
}
