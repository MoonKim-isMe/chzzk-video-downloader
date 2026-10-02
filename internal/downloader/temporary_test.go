package downloader

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTemporaryDownloadDirScopesByVideoNo(t *testing.T) {
	outputDir := filepath.Join(t.TempDir(), "downloads")
	tempDir, err := TemporaryDownloadDir(outputDir, 12345)
	if err != nil {
		t.Fatal(err)
	}

	expected := filepath.Join(outputDir, ".chzzk-temp", "12345")
	if tempDir != expected {
		t.Fatalf("unexpected temporary directory: %s", tempDir)
	}
}

func TestCleanupTemporaryDownloadRemovesOnlyTargetVideo(t *testing.T) {
	outputDir := t.TempDir()
	first, err := TemporaryDownloadDir(outputDir, 1001)
	if err != nil {
		t.Fatal(err)
	}
	second, err := TemporaryDownloadDir(outputDir, 1002)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(first, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(second, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(first, "stale.part"), []byte("first"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(second, "keep.part"), []byte("second"), 0o644); err != nil {
		t.Fatal(err)
	}
	finalPath := filepath.Join(outputDir, "completed.mp4")
	if err := os.WriteFile(finalPath, []byte("final"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := CleanupTemporaryDownload(outputDir, 1001); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(first); !os.IsNotExist(err) {
		t.Fatalf("target temporary directory still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(second, "keep.part")); err != nil {
		t.Fatalf("other VOD temporary file was removed: %v", err)
	}
	if _, err := os.Stat(finalPath); err != nil {
		t.Fatalf("final file was removed: %v", err)
	}
}


func TestTemporaryDownloadSizeTracksRegularFiles(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "video.part"), make([]byte, 128), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "audio.part"), make([]byte, 64), 0o644); err != nil {
		t.Fatal(err)
	}

	size, err := temporaryDownloadSize(root)
	if err != nil {
		t.Fatal(err)
	}
	if size != 192 {
		t.Fatalf("unexpected temporary size: %d", size)
	}
}

func TestTemporaryDownloadSizeMissingDirectoryIsZero(t *testing.T) {
	size, err := temporaryDownloadSize(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatal(err)
	}
	if size != 0 {
		t.Fatalf("unexpected missing directory size: %d", size)
	}
}


func TestDownloadAttemptTemporaryDirectoriesAreSeparated(t *testing.T) {
	outputDir := t.TempDir()
	root, err := TemporaryDownloadDir(outputDir, 15461111)
	if err != nil {
		t.Fatal(err)
	}
	nativeDir, err := nativeTemporaryDownloadDir(outputDir, 15461111)
	if err != nil {
		t.Fatal(err)
	}
	fallbackDir, err := fallbackTemporaryDownloadDir(outputDir, 15461111, "123456789")
	if err != nil {
		t.Fatal(err)
	}

	if nativeDir != filepath.Join(root, "native") {
		t.Fatalf("unexpected native temp dir: %q", nativeDir)
	}
	if fallbackDir != filepath.Join(root, "fallback-123456789") {
		t.Fatalf("unexpected fallback temp dir: %q", fallbackDir)
	}
	if nativeDir == fallbackDir {
		t.Fatalf("native and fallback temp directories must differ: %q", nativeDir)
	}
}
