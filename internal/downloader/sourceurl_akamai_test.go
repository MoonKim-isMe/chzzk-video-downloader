package downloader

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const sourceURLAkamaiFixture = `<?xml version="1.0" encoding="UTF-8"?>
<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" xmlns:nvod="urn:naver:vod:2020">
<Period>
<AdaptationSet mimeType="video/mp2t">
<Representation id="4bc9b54b-d63f-11f1-89b4-48df37e3021a" bandwidth="8196000" height="1080" codecs="avc1.64002a,mp4a.40.2"
 nvod:m3u="https://a01-g-naver-vod.akamaized.net/vod/hls/1080.m3u8?__gda__=old">
  <BaseURL>https://a01-g-naver-vod.akamaized.net/vod/hls/</BaseURL>
  <SegmentTemplate media="$RepresentationID$-$Number%06d$.ts?__gda__=old" startNumber="0" timescale="1000">
    <SegmentTimeline><S t="0" d="4000" r="2"/><S t="12000" d="750"/></SegmentTimeline>
  </SegmentTemplate>
</Representation>
<Representation id="720-stream" bandwidth="3194000" height="720" codecs="avc1.4d4020,mp4a.40.2"
 nvod:m3u="https://a01-g-naver-vod.akamaized.net/vod/hls/720.m3u8?__gda__=old">
  <BaseURL>https://a01-g-naver-vod.akamaized.net/vod/hls/</BaseURL>
  <SegmentTemplate media="$RepresentationID$-$Number%06d$.ts?__gda__=old" startNumber="0" timescale="1000">
    <SegmentTimeline><S t="0" d="4000" r="2"/></SegmentTimeline>
  </SegmentTemplate>
</Representation>
</AdaptationSet></Period></MPD>`

func TestSourceURLAkamaiSelectionAndLegacySupport(t *testing.T) {
	address, quality, err := selectSourceURLHLSForType([]byte(sourceURLAkamaiFixture), "bv*+ba/b", DownloadTypeVideo)
	if err != nil || quality != 1080 || !strings.Contains(address, "/1080.m3u8?__gda__=") {
		t.Fatalf("Akamai 1080p source unavailable: quality=%d err=%v", quality, err)
	}
	low, q, err := selectSourceURLHLSForType([]byte(sourceURLAkamaiFixture), "bv*[height<=720]+ba/b[height<=720]", DownloadTypeVideo)
	if err != nil || q != 720 || !strings.Contains(low, "/720.m3u8") {
		t.Fatalf("Akamai quality limitation broken: %d %v", q, err)
	}
	mp3, _, err := selectSourceURLHLSForType([]byte(sourceURLAkamaiFixture), "bestaudio/best", DownloadTypeAudio)
	if err != nil || mp3 != address {
		t.Fatalf("MP3 must reuse muxed Akamai video+audio: %v", err)
	}
	legacy, _, err := selectSourceURLHLS([]byte(sourceURLFixture), "bv*+ba/b")
	if err != nil || !strings.Contains(legacy, ".pstatic.net") {
		t.Fatalf("legacy pstatic HLS selection broken: %v", err)
	}
}

func TestSourceURLAkamaiAcceptsOnlyNAVERCDN(t *testing.T) {
	for _, raw := range []string{
		"https://a01-g-naver-vod.akamaized.net/hls/a.m3u8?__gda__=valid",
		"https://b03-g-naver-vod.akamaized.net/hls/a.m3u8?__gda__=valid",
	} {
		if !validSourceURLHLSAddress(raw) {
			t.Fatalf("trusted Akamai CDN rejected: %q", raw)
		}
	}
	for _, raw := range []string{
		"https://evil.akamaized.net/hls/a.m3u8?__gda__=valid",
		"https://a01-g-naver-vod.akamaized.net.evil.example/hls/a.m3u8?__gda__=valid",
		"http://a01-g-naver-vod.akamaized.net/hls/a.m3u8?__gda__=valid",
		"https://user:pass@a01-g-naver-vod.akamaized.net/hls/a.m3u8?__gda__=valid",
		"https://a01-g-naver-vod.akamaized.net:8443/hls/a.m3u8?__gda__=valid",
		"https://a01-g-naver-vod.akamaized.net/hls/a.ts?__gda__=valid",
	} {
		if validSourceURLHLSAddress(raw) {
			t.Fatalf("unsafe Akamai playlist accepted: %q", raw)
		}
	}
}

func TestSourceURLAkamaiAuthQueryPropagation(t *testing.T) {
	parent, _ := url.Parse("https://a01-g-naver-vod.akamaized.net/hls/master.m3u8?__gda__=old&other=secret")
	child, err := signedHLSChildURL(parent, "segment-000000.ts")
	if err != nil || child.Query().Get("__gda__") != "old" || child.Query().Has("other") {
		t.Fatalf("Akamai segment token was not inherited correctly: %v err=%v", child, err)
	}
	if sourceURLHLSAuthQueryKey(child) != "__gda__" {
		t.Fatal("Akamai signature key not recognized")
	}
	existing, err := signedHLSChildURL(parent, "segment-000001.ts?__gda__=special")
	if err != nil || existing.Query().Get("__gda__") != "special" {
		t.Fatal("child signature must not be replaced by playlist signature")
	}
	legacy, _ := url.Parse("https://cdn.pstatic.net/playlist.m3u8?_lsu_sa_=legacy&other=secret")
	legacyChild, err := signedHLSChildURL(legacy, "part.ts")
	if err != nil || sourceURLHLSAuthQueryKey(legacyChild) != "_lsu_sa_" ||
		legacyChild.Query().Get("_lsu_sa_") != "legacy" || legacyChild.Query().Has("other") {
		t.Fatalf("legacy CDN signature propagation regressed: %v, %v", legacyChild, err)
	}
}

func TestBuildSourceURLMPDPlaylistFromHARShape(t *testing.T) {
	selected, _, err := selectSourceURLHLS([]byte(sourceURLAkamaiFixture), "bv*+ba/b")
	if err != nil {
		t.Fatal(err)
	}
	content, err := buildSourceURLMPDHLSPlaylist([]byte(sourceURLAkamaiFixture), selected)
	if err != nil {
		t.Fatal(err)
	}
	playlist := string(content)
	for _, needle := range []string{
		"#EXTM3U\n", "#EXT-X-TARGETDURATION:4", "#EXT-X-MEDIA-SEQUENCE:0",
		"#EXTINF:4.000,", "#EXTINF:0.750,", "-000000.ts?__gda__=old",
		"-000001.ts?__gda__=old", "-000002.ts?__gda__=old",
		"-000003.ts?__gda__=old", "#EXT-X-ENDLIST",
	} {
		if !strings.Contains(playlist, needle) {
			t.Errorf("missing MPD playlist fragment %q", needle)
		}
	}
	if strings.Count(playlist, "#EXTINF:") != 4 {
		t.Fatalf("incorrect MPD segment count: %d", strings.Count(playlist, "#EXTINF:"))
	}
	if len(content) > maxSignedHLSPlaylistBytes {
		t.Fatal("generated HLS exceeds existing playlist limit")
	}
}

func TestBuildSourceURLMPDPlaylistWithLongVOD(t *testing.T) {
	// The supplied 15977157 HAR has 7291 four-second segments and a
	// short final segment in the 1080p SegmentTimeline.
	longManifest := strings.Replace(sourceURLAkamaiFixture, `r="2"`, `r="7290"`, 1)
	selected, _, _ := selectSourceURLHLS([]byte(longManifest), "bv*+ba/b")
	generated, err := buildSourceURLMPDHLSPlaylist([]byte(longManifest), selected)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(generated), "#EXTINF:") != 7292 ||
		!strings.Contains(string(generated), "-007291.ts?__gda__=old") {
		t.Fatalf("long VOD timeline incomplete: length=%d err=%v", len(generated), err)
	}
}

func TestSourceURLAkamaiMPDPlaylistFallbackAndSignatureRefresh(t *testing.T) {
	selected, _, _ := selectSourceURLHLS([]byte(sourceURLAkamaiFixture), "bv*+ba/b")
	generated, err := buildSourceURLMPDHLSPlaylist([]byte(sourceURLAkamaiFixture), selected)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	calls, renewals := 0, 0
	client := &http.Client{Transport: signedProxyTransport(func(req *http.Request) (*http.Response, error) {
		if strings.HasSuffix(req.URL.Path, ".m3u8") {
			return signedProxyResponse(req, http.StatusNotFound, ""), nil
		}
		if strings.HasSuffix(req.URL.Path, ".ts") {
			mu.Lock()
			calls++
			mu.Unlock()
			if req.URL.Query().Get("__gda__") == "old" {
				return signedProxyResponse(req, http.StatusForbidden, ""), nil
			}
			if req.URL.Query().Get("__gda__") == "new" {
				return signedProxyResponse(req, http.StatusOK, "a valid transport stream"), nil
			}
			return signedProxyResponse(req, http.StatusUnauthorized, ""), nil
		}
		return nil, fmt.Errorf("unexpected request path")
	})}
	folder := t.TempDir()
	source := sourceURLHLSStream{URL: selected, MPDPlaylist: generated}
	path, err := prepareSourceURLHLSPlaylist(context.Background(), client, source, folder)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "playlist-01.m3u8" {
		t.Fatalf("wrong generated path %q", path)
	}
	proxy, err := startSourceURLSegmentProxy(context.Background(), selected, folder,
		func(context.Context) (string, error) {
			mu.Lock()
			renewals++
			mu.Unlock()
			return strings.Replace(selected, "__gda__=old", "__gda__=new", 1), nil
		}, client)
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()
	local, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(local), "__gda__") {
		t.Fatal("secret signed CDN URI leaked into local FFmpeg playlist")
	}
	lines := strings.Split(string(local), "\n")
	var uri string
	for _, line := range lines {
		if strings.HasPrefix(line, "http://127.0.0.1:") {
			uri = line
			break
		}
	}
	if uri == "" {
		t.Fatal("segment proxy route missing")
	}
	resp, err := http.Get(uri)
	if err != nil {
		t.Fatal(err)
	}
	contents, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	mu.Lock()
	defer mu.Unlock()
	if resp.StatusCode != http.StatusOK || string(contents) != "a valid transport stream" ||
		calls != 2 || renewals != 1 {
		t.Fatalf("Akamai segment renewal failed: http=%d calls=%d renewals=%d", resp.StatusCode, calls, renewals)
	}
}

func TestSourceURLAkamaiResumeCacheIgnoresGDA(t *testing.T) {
	f1 := filepath.Join(t.TempDir(), "old")
	f2 := filepath.Join(t.TempDir(), "new")
	for _, tt := range []struct{folder, token string}{{f1, "old"}, {f2, "new"}} {
		if err := os.MkdirAll(tt.folder, 0700); err != nil {
			t.Fatal(err)
		}
		content := "#EXTM3U\n#EXTINF:4,\nhttps://a01-g-naver-vod.akamaized.net/hls/part.ts?__gda__=" + tt.token + "\n"
		if err := os.WriteFile(filepath.Join(tt.folder, "playlist-01.m3u8"), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	root := t.TempDir()
	oldCache, err1 := sourceURLSegmentCacheDir(root, 15977157, 1080, f1)
	newCache, err2 := sourceURLSegmentCacheDir(root, 15977157, 1080, f2)
	if err1 != nil || err2 != nil || oldCache != newCache {
		t.Fatalf("Akamai cache not stable across credentials: %s %s %v %v", oldCache, newCache, err1, err2)
	}
	oldSegment, _ := url.Parse("https://a01-g-naver-vod.akamaized.net/hls/part.ts?__gda__=old")
	newSegment, _ := url.Parse("https://a01-g-naver-vod.akamaized.net/hls/part.ts?__gda__=new")
	if sourceURLSegmentCacheFile(root, oldSegment) != sourceURLSegmentCacheFile(root, newSegment) {
		t.Fatal("Akamai media cache identity still depends on signature")
	}
}
