package downloader

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func writeSourceURLCacheFixture(t *testing.T, folder, signature, duration string) string {
	t.Helper()
	if err := os.MkdirAll(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	playlist := "#EXTM3U\n#EXTINF:" + duration + ",\nhttps://cdn.pstatic.net/hls/part-000001.ts?_lsu_sa_=" + signature + "\n#EXT-X-ENDLIST\n"
	path := filepath.Join(folder, "playlist-01.m3u8")
	if err := os.WriteFile(path, []byte(playlist), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSourceURLSegmentCacheKeySurvivesSignatureRenewal(t *testing.T) {
	root := t.TempDir()
	m1 := filepath.Join(root, "manifest-old")
	m2 := filepath.Join(root, "manifest-new")
	writeSourceURLCacheFixture(t, m1, "oldSignedValue", "4.0")
	writeSourceURLCacheFixture(t, m2, "newSignedValue", "4.0")
	c1, err := sourceURLSegmentCacheDir(root, 15651133, 1080, m1)
	if err != nil {
		t.Fatal(err)
	}
	c2, err := sourceURLSegmentCacheDir(root, 15651133, 1080, m2)
	if err != nil {
		t.Fatal(err)
	}
	if c1 != c2 || !strings.Contains(c1, sourceURLResumeDirectoryName) {
		t.Fatalf("signing a fresh URL must preserve segment cache: old=%s new=%s", c1, c2)
	}
	if strings.Contains(c1, "SignedValue") {
		t.Fatal("resume path must not contain the playback signature")
	}
	differentQuality, err := sourceURLSegmentCacheDir(root, 15651133, 720, m2)
	if err != nil || differentQuality == c1 {
		t.Fatalf("different quality must not reuse cache: %s", differentQuality)
	}
	differentVideo, err := sourceURLSegmentCacheDir(root, 15651134, 1080, m2)
	if err != nil || differentVideo == c1 {
		t.Fatalf("different video must not reuse cache: %s", differentVideo)
	}
	writeSourceURLCacheFixture(t, m2, "newSignedValue", "5.0")
	changedManifest, err := sourceURLSegmentCacheDir(root, 15651133, 1080, m2)
	if err != nil || changedManifest == c1 {
		t.Fatalf("changed HLS timeline must not reuse cache: %s", changedManifest)
	}
}

func TestSourceURLSegmentCacheReusesPreviouslyDownloadedSegments(t *testing.T) {
	root := t.TempDir()
	oldManifest := filepath.Join(root, "first")
	newManifest := filepath.Join(root, "second")
	oldPath := writeSourceURLCacheFixture(t, oldManifest, "expired", "4.0")
	newPath := writeSourceURLCacheFixture(t, newManifest, "renewed", "4.0")
	cache1, err := sourceURLSegmentCacheDir(root, 15651133, 1080, oldManifest)
	if err != nil {
		t.Fatal(err)
	}
	cache2, err := sourceURLSegmentCacheDir(root, 15651133, 1080, newManifest)
	if err != nil || cache1 != cache2 {
		t.Fatalf("cache continuity lost: old=%q new=%q err=%v", cache1, cache2, err)
	}
	var calls atomic.Int64
	firstClient := &http.Client{Transport: signedProxyTransport(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		return signedProxyResponse(req, 200, "media-segment-contents"), nil
	})}
	p1, err := startSourceURLSegmentProxy(
		context.Background(), "https://cdn.pstatic.net/hls/master.m3u8?_lsu_sa_=expired",
		oldManifest, func(context.Context) (string, error) { return "", nil }, firstClient, cache1,
	)
	if err != nil {
		t.Fatal(err)
	}
	data1, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	uri1 := strings.Split(string(data1), "\n")[2]
	response1, err := http.Get(uri1)
	if err != nil {
		t.Fatal(err)
	}
	body1, _ := io.ReadAll(response1.Body)
	response1.Body.Close()
	p1.Close()
	if response1.StatusCode != 200 || string(body1) != "media-segment-contents" || calls.Load() != 1 {
		t.Fatalf("initial segment download failed: status=%d calls=%d", response1.StatusCode, calls.Load())
	}

	// Simulate another application run. CDN authorization changes, and this
	// backend is deliberately unavailable: the segment must be read from disk.
	secondClient := &http.Client{Transport: signedProxyTransport(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		return signedProxyResponse(req, http.StatusForbidden, ""), nil
	})}
	p2, err := startSourceURLSegmentProxy(
		context.Background(), "https://cdn.pstatic.net/hls/master.m3u8?_lsu_sa_=renewed",
		newManifest, func(context.Context) (string, error) { return "", nil }, secondClient, cache2,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer p2.Close()
	data2, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatal(err)
	}
	uri2 := strings.Split(string(data2), "\n")[2]
	response2, err := http.Get(uri2)
	if err != nil {
		t.Fatal(err)
	}
	body2, _ := io.ReadAll(response2.Body)
	response2.Body.Close()
	if response2.StatusCode != 200 || string(body2) != string(body1) || calls.Load() != 1 {
		t.Fatalf("cached media re-fetched from CDN: status=%d calls=%d", response2.StatusCode, calls.Load())
	}
	if !strings.Contains(p2.Stats(), "cached_segments=1") || !strings.Contains(p2.Stats(), "reused_bytes=") {
		t.Fatalf("resume statistics not recorded: %s", p2.Stats())
	}
}

func TestSourceURLCacheNeverPersistsKeysOrPartialMedia(t *testing.T) {
	dir := t.TempDir()
	segment, err := url.Parse("https://cdn.pstatic.net/hls/part.ts?_lsu_sa_=secret")
	if err != nil {
		t.Fatal(err)
	}
	key, err := url.Parse("https://cdn.pstatic.net/hls/encryption.key?_lsu_sa_=secret")
	if err != nil {
		t.Fatal(err)
	}
	if !sourceURLCacheableMediaURL(segment) || sourceURLCacheableMediaURL(key) {
		t.Fatal("only media segments can be cached")
	}
	cacheFile := sourceURLSegmentCacheFile(dir, segment)
	if err := os.WriteFile(cacheFile+".partial", []byte("incomplete"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, found := readSourceURLSegmentCache(cacheFile); found {
		t.Fatal("partial segment was mistaken for completed cached media")
	}
	if err := writeSourceURLSegmentCache(dir, cacheFile, []byte("verified full segment")); err != nil {
		t.Fatal(err)
	}
	if recovered, found := readSourceURLSegmentCache(cacheFile); !found || string(recovered) != "verified full segment" {
		t.Fatalf("atomic segment persistence failed: found=%t content=%q", found, recovered)
	}
	if strings.Contains(cacheFile, "secret") {
		t.Fatal("signed authentication query exposed in cache filename")
	}
}
