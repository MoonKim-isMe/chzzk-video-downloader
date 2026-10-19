package downloader

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func makeParallelHLSManifest(t *testing.T, count int) string {
	t.Helper()
	dir := t.TempDir()
	var body strings.Builder
	body.WriteString("#EXTM3U\n#EXT-X-VERSION:3\n")
	for i := 0; i < count; i++ {
		fmt.Fprintf(&body, "#EXTINF:4,\nhttps://cdn.pstatic.net/hls/part-%03d.ts?_lsu_sa_=test\n", i)
	}
	body.WriteString("#EXT-X-ENDLIST\n")
	if err := os.WriteFile(filepath.Join(dir, "playlist-01.m3u8"), []byte(body.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func countCachedHLSSegments(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".segment") {
			count++
		}
	}
	return count
}

func TestSourceURLPrefetchUsesAccelerationPerDownload(t *testing.T) {
	for _, limit := range []int{1, 2, 4, 8} {
		t.Run(fmt.Sprintf("workers_%d", limit), func(t *testing.T) {
			const segments = 16
			manifest := makeParallelHLSManifest(t, segments)
			cache := t.TempDir()
			var active, peak, calls atomic.Int64
			client := &http.Client{Transport: signedProxyTransport(func(req *http.Request) (*http.Response, error) {
				calls.Add(1)
				current := active.Add(1)
				for {
					old := peak.Load()
					if old >= current || peak.CompareAndSwap(old, current) {
						break
					}
				}
				defer active.Add(-1)
				select {
				case <-time.After(30 * time.Millisecond):
				case <-req.Context().Done():
					return nil, req.Context().Err()
				}
				return signedProxyResponse(req, http.StatusOK, "downloaded chunk"), nil
			})}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			p, err := startSourceURLSegmentProxyWithAcceleration(ctx,
				"https://cdn.pstatic.net/hls/playlist-01.m3u8?_lsu_sa_=test",
				manifest,
				func(context.Context) (string, error) { return "", fmt.Errorf("unexpected renewal") },
				client, cache, limit,
			)
			if err != nil {
				t.Fatal(err)
			}
			defer p.Close()
			if limit == 1 {
				time.Sleep(50 * time.Millisecond)
				if calls.Load() != 0 {
					t.Fatal("stable (1) must preserve sequential, on-demand fetching")
				}
				data, err := os.ReadFile(filepath.Join(manifest, "playlist-01.m3u8"))
				if err != nil {
					t.Fatal(err)
				}
				url := strings.Split(string(data), "\n")[3]
				res, err := http.Get(url)
				if err != nil {
					t.Fatal(err)
				}
				_, _ = io.Copy(io.Discard, res.Body)
				res.Body.Close()
				if res.StatusCode != http.StatusOK {
					t.Fatalf("on-demand segment failed: HTTP %d", res.StatusCode)
				}
			} else {
				deadline := time.Now().Add(5 * time.Second)
				for countCachedHLSSegments(t, cache) != segments && time.Now().Before(deadline) {
					time.Sleep(10 * time.Millisecond)
				}
				if got := countCachedHLSSegments(t, cache); got != segments {
					t.Fatalf("prefetch stored %d/%d segments; %s", got, segments, p.Stats())
				}
			}
			p.Close()
			if got := peak.Load(); got > int64(limit) || (limit > 1 && got < 2) {
				t.Fatalf("acceleration=%d actual parallel requests=%d", limit, got)
			}
			if calls.Load() != int64(segments) && limit > 1 {
				t.Fatalf("unnecessary CDN calls: got %d, want %d", calls.Load(), segments)
			}
			if !strings.Contains(p.Stats(), fmt.Sprintf("cdn_parallel_limit=%d", limit)) {
				t.Fatalf("missing per-download acceleration in diagnostics: %s", p.Stats())
			}
		})
	}
}

func TestSourceURLPrefetchSharesInflightWithFFmpeg(t *testing.T) {
	manifest := makeParallelHLSManifest(t, 1)
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var calls atomic.Int64
	client := &http.Client{Transport: signedProxyTransport(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		select {
		case started <- struct{}{}:
		default:
		}
		select {
		case <-release:
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
		return signedProxyResponse(req, http.StatusOK, "shared segment"), nil
	})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p, err := startSourceURLSegmentProxyWithAcceleration(ctx,
		"https://cdn.pstatic.net/hls/playlist-01.m3u8?_lsu_sa_=test",
		manifest, func(context.Context) (string, error) { return "", nil }, client, t.TempDir(), 2)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("prefetch was not started")
	}
	data, err := os.ReadFile(filepath.Join(manifest, "playlist-01.m3u8"))
	if err != nil {
		t.Fatal(err)
	}
	url := strings.Split(string(data), "\n")[3]
	done := make(chan error, 1)
	go func() {
		res, err := http.Get(url)
		if err != nil {
			done <- err
			return
		}
		body, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			done <- err
			return
		}
		if res.StatusCode != 200 || string(body) != "shared segment" {
			done <- fmt.Errorf("unexpected FFmpeg response: HTTP %d body %q", res.StatusCode, body)
			return
		}
		done <- nil
	}()
	time.Sleep(25 * time.Millisecond)
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("sequential FFmpeg consumer blocked by prefetch")
	}
	p.Close()
	if calls.Load() != 1 {
		t.Fatalf("same HLS segment downloaded more than once: %d", calls.Load())
	}
}

func TestSourceURLTwoConcurrentJobsKeepIndependentLimits(t *testing.T) {
	var inflight, peak atomic.Int64
	gate := make(chan struct{})
	var fourth atomic.Int64
	client := &http.Client{Transport: signedProxyTransport(func(req *http.Request) (*http.Response, error) {
		count := inflight.Add(1)
		for {
			highest := peak.Load()
			if highest >= count || peak.CompareAndSwap(highest, count) {
				break
			}
		}
		defer inflight.Add(-1)
		if fourth.Add(1) == 4 {
			close(gate)
		}
		select {
		case <-gate:
		case <-time.After(2 * time.Second):
			return nil, fmt.Errorf("four independent CDN requests did not start")
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
		return signedProxyResponse(req, 200, "concurrent chunk"), nil
	})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var proxies []*sourceURLSegmentProxy
	for i := 0; i < 2; i++ {
		p, err := startSourceURLSegmentProxyWithAcceleration(ctx,
			"https://cdn.pstatic.net/hls/playlist-01.m3u8?_lsu_sa_=test",
			makeParallelHLSManifest(t, 8),
			func(context.Context) (string, error) { return "", fmt.Errorf("unexpected renewal") },
			client, t.TempDir(), 2,
		)
		if err != nil {
			t.Fatal(err)
		}
		proxies = append(proxies, p)
		defer p.Close()
	}
	select {
	case <-gate:
	case <-time.After(3 * time.Second):
		t.Fatal("two downloads at acceleration 2 did not start four CDN requests")
	}
	for _, proxy := range proxies {
		proxy.Close()
	}
	if peak.Load() != 4 {
		t.Fatalf("expected 2 requests per job, four combined: max=%d", peak.Load())
	}
}
