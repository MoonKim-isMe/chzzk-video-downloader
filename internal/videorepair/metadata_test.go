package videorepair

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileDialogPatternContainsSupportedExtensions(t *testing.T) {
	pattern := FileDialogPattern()
	for _, extension := range SupportedVideoExtensions() {
		if !strings.Contains(pattern, "*"+extension) {
			t.Fatalf("missing extension %s in pattern %q", extension, pattern)
		}
	}
}

func TestIsSupportedPath(t *testing.T) {
	for _, path := range []string{"sample.mp4", "sample.MKV", "sample.webm", "sample.m2ts"} {
		if !IsSupportedPath(path) {
			t.Fatalf("expected supported path: %s", path)
		}
	}
	if IsSupportedPath("sample.txt") {
		t.Fatal("text file must not be supported")
	}
}

func TestParseProbeOutput(t *testing.T) {
	formatName, duration, err := parseProbeOutput([]byte(`{"format":{"format_name":"mov,mp4,m4a,3gp,3g2,mj2","duration":"21600.125"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if formatName != "mov,mp4,m4a,3gp,3g2,mj2" {
		t.Fatalf("unexpected format name: %s", formatName)
	}
	if math.Abs(duration-21600.125) > 0.0001 {
		t.Fatalf("unexpected duration: %f", duration)
	}
	if container := normalizeContainer(formatName, ".mp4"); container != "MP4" {
		t.Fatalf("unexpected container: %s", container)
	}
}

func TestProbeFileKeepsBasicInfoWhenMetadataProbeUnavailable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken-video.mp4")
	if err := os.WriteFile(path, []byte("not-a-real-video"), 0o644); err != nil {
		t.Fatal(err)
	}

	info, err := ProbeFile(context.Background(), "", path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "broken-video.mp4" {
		t.Fatalf("unexpected name: %s", info.Name)
	}
	if info.ModifiedUnixMilli <= 0 {
		t.Fatalf("file modification signature was not captured: %#v", info)
	}
	if info.Container != "MP4" || info.ContainerSource != "extension" {
		t.Fatalf("unexpected fallback container: %#v", info)
	}
	if info.MetadataAvailable {
		t.Fatal("metadata must not be marked available without ffprobe")
	}
	if strings.TrimSpace(info.MetadataError) == "" {
		t.Fatal("metadata fallback reason must be present")
	}
}

func TestBasicFileInfoRejectsUnsupportedExtension(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(path, []byte("text"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := basicFileInfo(path)
	if err == nil {
		t.Fatal("expected unsupported extension error")
	}
}
