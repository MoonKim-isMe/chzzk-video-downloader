package downloader

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type signedProxyTransport func(*http.Request) (*http.Response, error)

func (f signedProxyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func signedProxyResponse(req *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status, Body: io.NopCloser(strings.NewReader(body)),
		Header: make(http.Header), Request: req,
	}
}

func TestSourceURLProxyRefreshesExpiredSegmentSignature(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "playlist-01.m3u8")
	initial := "https://cdn.pstatic.net/hls/main.m3u8?_lsu_sa_=old"
	if err := os.WriteFile(manifest, []byte("#EXTM3U\n#EXTINF:4.0,\nhttps://cdn.pstatic.net/hls/segment-01.ts?_lsu_sa_=old\n#EXT-X-ENDLIST\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var segments, renewals int
	client := &http.Client{Transport: signedProxyTransport(func(req *http.Request) (*http.Response, error) {
		mu.Lock()
		defer mu.Unlock()
		segments++
		if req.URL.Query().Get("_lsu_sa_") == "old" {
			return signedProxyResponse(req, http.StatusForbidden, "expired"), nil
		}
		if req.URL.Query().Get("_lsu_sa_") == "new" {
			return signedProxyResponse(req, http.StatusOK, "valid transport stream"), nil
		}
		return signedProxyResponse(req, http.StatusUnauthorized, "missing signature"), nil
	})}
	proxy, err := startSourceURLSegmentProxy(context.Background(), initial, dir, func(context.Context) (string, error) {
		mu.Lock()
		renewals++
		mu.Unlock()
		return "https://cdn.pstatic.net/hls/main.m3u8?_lsu_sa_=new", nil
	}, client)
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()
	b, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(b), "\n")
	uri := lines[2]
	if !strings.HasPrefix(uri, "http://127.0.0.1:") || !strings.HasSuffix(uri, ".ts") || strings.Contains(string(b), "_lsu_sa_") {
		t.Fatalf("manifest contains original signed URI: %s", b)
	}
	resp, err := http.Get(uri)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	got, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || string(got) != "valid transport stream" {
		t.Fatalf("unexpected segment result %d: %s", resp.StatusCode, got)
	}
	mu.Lock()
	defer mu.Unlock()
	if renewals != 1 || segments != 2 {
		t.Fatalf("wanted 1 refresh and 2 attempts, got refreshes=%d segments=%d", renewals, segments)
	}
	if !strings.Contains(proxy.Stats(), "auth_refreshes=1") {
		t.Fatalf("refresh not reported: %s", proxy.Stats())
	}
}

func TestSourceURLProxyRetriesTransientNetworkFailures(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "playlist-01.m3u8"), []byte("#EXTM3U\n#EXTINF:4,\nhttps://cdn.pstatic.net/hls/chunk.ts?_lsu_sa_=valid\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	attempts := 0
	client := &http.Client{Transport: signedProxyTransport(func(req *http.Request) (*http.Response, error) {
		mu.Lock()
		attempts++
		current := attempts
		mu.Unlock()
		if current == 1 {
			return nil, fmt.Errorf("simulated network timeout")
		}
		return signedProxyResponse(req, 200, "chunk"), nil
	})}
	proxy, err := startSourceURLSegmentProxy(context.Background(), "https://cdn.pstatic.net/hls/master.m3u8?_lsu_sa_=valid", dir, func(context.Context) (string, error) {
		t.Error("must not refresh token for one temporary failure")
		return "", fmt.Errorf("unexpected refresh")
	}, client)
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()
	b, _ := os.ReadFile(filepath.Join(dir, "playlist-01.m3u8"))
	uri := strings.Split(string(b), "\n")[2]
	resp, err := http.Get(uri)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(resp.Body)
	mu.Lock()
	defer mu.Unlock()
	if resp.StatusCode != 200 || string(payload) != "chunk" || attempts != 2 {
		t.Fatalf("retry result status=%d data=%s attempts=%d", resp.StatusCode, payload, attempts)
	}
}

func TestSourceURLProxyRejectsUnsignedManifestAndForeignResource(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "playlist-01.m3u8")
	if err := os.WriteFile(manifest, []byte("#EXTM3U\nhttps://evil.example/data.ts?_lsu_sa_=secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	renew := func(context.Context) (string, error) { return "", nil }
	client := &http.Client{}
	if _, err := startSourceURLSegmentProxy(context.Background(), "https://cdn.pstatic.net/a.m3u8", dir, renew, client); err == nil {
		t.Fatal("accepted unsigned manifest")
	}
	if _, err := startSourceURLSegmentProxy(context.Background(), "https://cdn.pstatic.net/a.m3u8?_lsu_sa_=secret", dir, renew, client); err == nil {
		t.Fatal("accepted foreign HLS resource")
	}
}

func TestSourceURLProxyDetectsUnrecoverableSegment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "playlist-01.m3u8")
	if err := os.WriteFile(path, []byte("#EXTM3U\n#EXTINF:4,\nhttps://cdn.pstatic.net/hls/segment.ts?_lsu_sa_=expired\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: signedProxyTransport(func(req *http.Request) (*http.Response, error) {
		return signedProxyResponse(req, http.StatusForbidden, ""), nil
	})}
	proxy, err := startSourceURLSegmentProxy(context.Background(), "https://cdn.pstatic.net/hls/main.m3u8?_lsu_sa_=expired", dir, func(context.Context) (string, error) {
		return "", fmt.Errorf("renewal failed")
	}, client)
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()
	content, _ := os.ReadFile(path)
	uri := strings.Split(string(content), "\n")[2]
	resp, err := http.Get(uri)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway || !proxy.HasFailedSegments() {
		t.Fatalf("unrecoverable segment must prevent completed status: http=%d stats=%s", resp.StatusCode, proxy.Stats())
	}
	if strings.Contains(proxy.Stats(), "expired") {
		t.Fatal("signed playback credential leaked in diagnostics")
	}
}
