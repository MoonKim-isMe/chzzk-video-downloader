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
