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
	"testing"
)

func signedHLSURL(raw string) *url.URL {
	result, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return result
}

func TestSignedHLSChildURLPropagatesOnlyKnownSignature(t *testing.T) {
	base := signedHLSURL("https://b01-kr-naver-vod.pstatic.net/path/master.m3u8?_lsu_sa_=ABCD&other=secret")
	tests := []struct {
		child string
		want  string
	}{
		{"part-000000.ts", "https://b01-kr-naver-vod.pstatic.net/path/part-000000.ts?_lsu_sa_=ABCD"},
		{"../part-000001.ts?x=1", "https://b01-kr-naver-vod.pstatic.net/part-000001.ts?_lsu_sa_=ABCD&x=1"},
		{"https://b01-kr-naver-vod.pstatic.net/path/part-000002.ts?_lsu_sa_=other", "https://b01-kr-naver-vod.pstatic.net/path/part-000002.ts?_lsu_sa_=other"},
		{"https://b02-kr-naver-vod.pstatic.net/path/part-000003.ts", "https://b02-kr-naver-vod.pstatic.net/path/part-000003.ts"},
	}
	for _, test := range tests {
		got, err := signedHLSChildURL(base, test.child)
		if err != nil || got.String() != test.want {
			t.Errorf("signedHLSChildURL(%q): got=%v, err=%v", test.child, got, err)
		}
	}
	for _, bad := range []string{
		"http://b01-kr-naver-vod.pstatic.net/x.ts", "https://evil.example/x.ts",
		"//evil.example/x.ts", "file:///etc/passwd", "https://user:pass@pstatic.net/key.key",
	} {
		if _, err := signedHLSChildURL(base, bad); err == nil {
			t.Errorf("unexpectedly allowed unsafe resource %q", bad)
		}
	}
}

type fakeHLSRoundTripper func(*http.Request) (*http.Response, error)

func (f fakeHLSRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestPrepareSignedHLSPlaylistWithNestedPlaylistAndKey(t *testing.T) {
	requested := []string{}
	fake := fakeHLSRoundTripper(func(req *http.Request) (*http.Response, error) {
		if req.URL.Query().Get("_lsu_sa_") != "ABCD" {
			return nil, fmt.Errorf("request without expected signature")
		}
		requested = append(requested, req.URL.Path)
		var body string
		switch req.URL.Path {
		case "/hls/master.m3u8":
			body = "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=300000\nchild.m3u8\n"
		case "/hls/child.m3u8":
			body = "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-KEY:METHOD=AES-128,URI=\"key.key\"\n#EXTINF:2.0,\nsegment-000000.ts\n#EXTINF:2.0,\nsegment-000001.ts?foo=bar\n#EXT-X-ENDLIST\n"
		default:
			return nil, fmt.Errorf("unexpected playlist fetch")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
	})
	dir := filepath.Join(t.TempDir(), "manifests")
	root, err := prepareSignedHLSPlaylist(context.Background(), &http.Client{Transport: fake}, "https://b01-kr-naver-vod.pstatic.net/hls/master.m3u8?_lsu_sa_=ABCD", dir)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(root)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "playlist-02.m3u8") || strings.Contains(string(data), "child.m3u8") {
		t.Fatalf("master rewrite incorrect: %s", data)
	}
	child, err := os.ReadFile(filepath.Join(dir, "playlist-02.m3u8"))
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{
		"segment-000000.ts?_lsu_sa_=ABCD", "segment-000001.ts?_lsu_sa_=ABCD&foo=bar",
		"URI=\"https://b01-kr-naver-vod.pstatic.net/hls/key.key?_lsu_sa_=ABCD\"",
	} {
		if !strings.Contains(string(child), marker) {
			t.Errorf("missing rewritten resource %s", marker)
		}
	}
	if len(requested) != 2 {
		t.Fatalf("expected exactly two playlists, got %d", len(requested))
	}
}

func TestPrepareSignedHLSPlaylistRejectsUnsignedForeignHostAndInvalidManifest(t *testing.T) {
	for _, body := range []string{
		"#EXTM3U\nhttps://evil.example/segment.ts\n",
		"#EXTM3U\nfile:///etc/passwd\n",
		"NOT_A_PLAYLIST\nsegment.ts\n",
	} {
		fake := fakeHLSRoundTripper(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
		})
		_, err := prepareSignedHLSPlaylist(context.Background(), &http.Client{Transport: fake}, "https://cdn.pstatic.net/x.m3u8?_lsu_sa_=ABCD", t.TempDir())
		if err == nil {
			t.Errorf("accepted invalid playlist %q", body)
		}
		if strings.Contains(err.Error(), "ABCD") {
			t.Errorf("leaked signature in error: %v", err)
		}
	}
}
