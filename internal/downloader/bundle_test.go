package downloader

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
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


func TestMaterializeBundledToolsSerializesConcurrentWrites(t *testing.T) {
	root := "bundle"
	data := []byte("yt-dlp-binary")
	manifest := bundledToolManifest{
		BundleID: "concurrent-bundle",
		Platform: "windows/amd64",
		Tools: []bundledToolFile{{
			Name:   "yt-dlp.exe",
			SHA256: hashBytes(data),
		}},
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	bundle := fstest.MapFS{
		root + "/" + bundleManifestName: {Data: manifestBytes},
		root + "/yt-dlp.exe":            {Data: data},
	}
	target := t.TempDir()

	const workers = 16
	start := make(chan struct{})
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	wg.Add(workers)
	for index := 0; index < workers; index++ {
		go func() {
			defer wg.Done()
			<-start
			errs <- materializeBundledTools(bundle, root, target)
		}()
	}
	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent materialization failed: %v", err)
		}
	}
	actual, err := os.ReadFile(filepath.Join(target, "yt-dlp.exe"))
	if err != nil {
		t.Fatal(err)
	}
	if string(actual) != string(data) {
		t.Fatalf("unexpected materialized tool: %q", actual)
	}
	marker, err := os.ReadFile(filepath.Join(target, bundleMarkerName))
	if err != nil {
		t.Fatal(err)
	}
	if string(marker) != manifest.BundleID+"\n" {
		t.Fatalf("unexpected bundle marker: %q", marker)
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

func TestPortableBundleReusesManagedToolAndRepairsMissingFile(t *testing.T) {
	data := []byte("bundled-tool")
	manifest := bundledToolManifest{
		BundleID: "portable-bundle",
		Platform: "windows/amd64",
		Tools:    []bundledToolFile{{Name: "yt-dlp.exe", SHA256: hashBytes(data)}},
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	bundle := fstest.MapFS{
		"bundle/" + bundleManifestName: {Data: manifestBytes},
		"bundle/yt-dlp.exe":            {Data: data},
	}
	target := filepath.Join(t.TempDir(), "user data", "CHZZK Video Downloader", "tools")
	if err := materializeBundledTools(bundle, "bundle", target); err != nil {
		t.Fatal(err)
	}
	toolPath := filepath.Join(target, "yt-dlp.exe")
	updated := []byte("user-updated-tool")
	if err := os.WriteFile(toolPath, updated, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := materializeBundledTools(bundle, "bundle", target); err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(toolPath)
	if err != nil || string(actual) != string(updated) {
		t.Fatalf("same bundle must preserve a managed tool across launches: %q, %v", actual, err)
	}
	if err := os.Remove(toolPath); err != nil {
		t.Fatal(err)
	}
	if err := materializeBundledTools(bundle, "bundle", target); err != nil {
		t.Fatal(err)
	}
	actual, err = os.ReadFile(toolPath)
	if err != nil || string(actual) != string(data) {
		t.Fatalf("missing managed tool must be restored from EXE: %q, %v", actual, err)
	}
}
