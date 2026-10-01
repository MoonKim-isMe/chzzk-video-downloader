package downloader

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestMaterializeBundledTools(t *testing.T) {
	root := "bundle"
	yt := []byte("yt-dlp-binary")
	ffmpeg := []byte("ffmpeg-binary")
	ffprobe := []byte("ffprobe-binary")

	manifest := bundledToolManifest{
		BundleID: "test-bundle-1",
		Platform: "windows/amd64",
		Tools: []bundledToolFile{
			{Name: "yt-dlp.exe", SHA256: hashBytes(yt)},
			{Name: "ffmpeg.exe", SHA256: hashBytes(ffmpeg)},
			{Name: "ffprobe.exe", SHA256: hashBytes(ffprobe)},
		},
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}

	bundle := fstest.MapFS{
		root + "/" + bundleManifestName: {Data: manifestBytes},
		root + "/yt-dlp.exe":            {Data: yt},
		root + "/ffmpeg.exe":            {Data: ffmpeg},
		root + "/ffprobe.exe":           {Data: ffprobe},
	}
	target := t.TempDir()

	if err := materializeBundledTools(bundle, root, target); err != nil {
		t.Fatal(err)
	}
	for _, item := range manifest.Tools {
		data, err := os.ReadFile(filepath.Join(target, item.Name))
		if err != nil {
			t.Fatal(err)
		}
		if hashBytes(data) != item.SHA256 {
			t.Fatalf("unexpected materialized hash for %s", item.Name)
		}
	}

	marker, err := os.ReadFile(filepath.Join(target, bundleMarkerName))
	if err != nil {
		t.Fatal(err)
	}
	if string(marker) != manifest.BundleID+"\n" {
		t.Fatalf("unexpected bundle marker: %q", marker)
	}

	if err := materializeBundledTools(bundle, root, target); err != nil {
		t.Fatalf("same bundle should be reusable: %v", err)
	}
}

func TestMaterializeBundledToolsReplacesChangedBundle(t *testing.T) {
	root := "bundle"
	first := []byte("first")
	second := []byte("second")
	target := t.TempDir()

	makeBundle := func(bundleID string, data []byte) fstest.MapFS {
		manifest := bundledToolManifest{
			BundleID: bundleID,
			Tools: []bundledToolFile{{
				Name:   "yt-dlp.exe",
				SHA256: hashBytes(data),
			}},
		}
		manifestBytes, _ := json.Marshal(manifest)
		return fstest.MapFS{
			root + "/" + bundleManifestName: {Data: manifestBytes},
			root + "/yt-dlp.exe":            {Data: data},
		}
	}

	if err := materializeBundledTools(makeBundle("first", first), root, target); err != nil {
		t.Fatal(err)
	}
	if err := materializeBundledTools(makeBundle("second", second), root, target); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(target, "yt-dlp.exe"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(second) {
		t.Fatalf("bundle was not replaced: %q", data)
	}
}

func TestMaterializeBundledToolsRejectsChecksumMismatch(t *testing.T) {
	root := "bundle"
	manifest := bundledToolManifest{
		BundleID: "bad",
		Tools: []bundledToolFile{{
			Name:   "yt-dlp.exe",
			SHA256: hashBytes([]byte("expected")),
		}},
	}
	manifestBytes, _ := json.Marshal(manifest)
	bundle := fstest.MapFS{
		root + "/" + bundleManifestName: {Data: manifestBytes},
		root + "/yt-dlp.exe":            {Data: []byte("tampered")},
	}

	if err := materializeBundledTools(bundle, root, t.TempDir()); err == nil {
		t.Fatal("expected checksum error")
	}
}

func TestMaterializeBundledToolsRequiresPreparedManifest(t *testing.T) {
	root := "bundle"
	bundle := fstest.MapFS{
		root + "/README.txt": {Data: []byte("placeholder")},
	}
	if err := materializeBundledTools(bundle, root, t.TempDir()); err == nil {
		t.Fatal("expected unavailable bundle error")
	}
}

func hashBytes(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
