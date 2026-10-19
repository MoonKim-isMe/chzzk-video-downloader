package downloader

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	sourceURLSegmentTimeout = 20 * time.Second
	sourceURLSegmentRetries = 3
	sourceURLSegmentMaxBytes = 64 << 20
)

type sourceURLSegmentProxy struct {
	mu               sync.Mutex
	// Parallel prefetch and FFmpeg must never fetch the same resource twice.
	segmentLocks     map[string]*sync.Mutex
	mediaURLs        []string
	downloadSlots    chan struct{}
	prefetchCancel   context.CancelFunc
	prefetchWorkers  sync.WaitGroup
	closeOnce        sync.Once
	refreshMu        sync.Mutex
	prefetchRequests int
	resources        map[string]*url.URL
	failedIDs        map[string]bool
	original         *url.URL
	current          *url.URL
	renew            func(context.Context) (string, error)
	client           *http.Client
	listener         net.Listener
	server           *http.Server
	closed           chan struct{}
	cacheDir         string
	cacheHits        int
	reusedBytes      int64
	cacheWrites      int
	cacheErrors      int
	prefix           string
	requests         int
	retries          int
	refreshes        int
	refreshAttempts  int
	lastStatus       int
}

func startSourceURLSegmentProxy(ctx context.Context, originalURL, manifestDir string, renew func(context.Context) (string, error), client *http.Client, cacheDir ...string) (*sourceURLSegmentProxy, error) {
	folder := ""
	if len(cacheDir) > 0 {
		folder = cacheDir[0]
	}
	// Existing clients use the original sequential behavior unless a specific
	// per-job acceleration limit is provided.
	return startSourceURLSegmentProxyWithAcceleration(ctx, originalURL, manifestDir, renew, client, folder, 1)
}

func startSourceURLSegmentProxyWithAcceleration(ctx context.Context, originalURL, manifestDir string, renew func(context.Context) (string, error), client *http.Client, cacheDir string, acceleration int) (*sourceURLSegmentProxy, error) {
	acceleration, err := normalizeConcurrentFragments(acceleration)
	if err != nil {
		return nil, err
	}
	original, err := url.Parse(originalURL)
	if err != nil || !validSourceURLHLSAddress(originalURL) || original.Query().Get("_lsu_sa_") == "" {
		return nil, fmt.Errorf("서명된 HLS 주소를 확인할 수 없습니다")
	}
	if client == nil || renew == nil {
		return nil, fmt.Errorf("HLS 세그먼트 연결 정보를 준비할 수 없습니다")
	}
	var persistentCache string
	if cacheDir != "" {
		persistentCache = cacheDir
		if !filepath.IsAbs(persistentCache) {
			return nil, fmt.Errorf("이어받기 저장 폴더는 절대 경로여야 합니다")
		}
		if err := os.MkdirAll(persistentCache, 0o700); err != nil {
			return nil, fmt.Errorf("이어받기 저장 폴더를 준비할 수 없습니다: %w", err)
		}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("HLS 세그먼트 중계 연결을 시작할 수 없습니다: %w", err)
	}
	var secret [16]byte
	if _, err := rand.Read(secret[:]); err != nil {
		listener.Close()
		return nil, fmt.Errorf("HLS 세그먼트 연결 보안 정보를 만들 수 없습니다")
	}
	proxy := &sourceURLSegmentProxy{
		resources:     make(map[string]*url.URL),
		segmentLocks: make(map[string]*sync.Mutex),
		downloadSlots: make(chan struct{}, acceleration),
		failedIDs:     make(map[string]bool),
		original:      original,
		current:       original,
		renew:         renew,
		client:        client,
		cacheDir:      persistentCache,
		listener:      listener,
		closed:        make(chan struct{}),
		prefix:        "/hls/" + hex.EncodeToString(secret[:]) + "/",
	}
	proxy.server = &http.Server{Handler: proxy, ReadHeaderTimeout: 5 * time.Second}
	if err := proxy.rewriteManifests(manifestDir); err != nil {
		listener.Close()
		return nil, err
	}
	go func() {
		_ = proxy.server.Serve(listener)
	}()
	proxy.startParallelPrefetch(ctx, acceleration)
	go func() {
		select {
		case <-ctx.Done():
			proxy.Close()
		case <-proxy.closed:
		}
	}()
	return proxy, nil
}

func (p *sourceURLSegmentProxy) Close() {
	p.closeOnce.Do(func() {
		close(p.closed)
		if p.prefetchCancel != nil {
			p.prefetchCancel()
		}
		_ = p.server.Close()
		p.prefetchWorkers.Wait()
	})
}

func (p *sourceURLSegmentProxy) Stats() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return fmt.Sprintf("HLS segment proxy: requests=%d retries=%d auth_refresh_attempts=%d auth_refreshes=%d failed_segments=%d last_http_status=%d cached_segments=%d reused_bytes=%d saved_segments=%d cache_errors=%d cdn_parallel_limit=%d prefetch_requests=%d", p.requests, p.retries, p.refreshAttempts, p.refreshes, len(p.failedIDs), p.lastStatus, p.cacheHits, p.reusedBytes, p.cacheWrites, p.cacheErrors, cap(p.downloadSlots), p.prefetchRequests)
}

func (p *sourceURLSegmentProxy) HasFailedSegments() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.failedIDs) > 0
}

func (p *sourceURLSegmentProxy) register(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || !validSignedHLSResource(u) || u.Query().Get("_lsu_sa_") == "" {
		return "", fmt.Errorf("허용되지 않은 서명 HLS 세그먼트 주소입니다")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	id := strconv.Itoa(len(p.resources) + 1)
	p.resources[id] = u
	p.segmentLocks[id] = &sync.Mutex{}
	// Keep the original resource extension so FFmpeg's HLS demuxer can
	// accept TS, fMP4 and key resources without allowing arbitrary files.
	local := "http://" + p.listener.Addr().String() + p.prefix + id + filepath.Ext(u.Path)
	if p.cacheDir != "" && sourceURLCacheableMediaURL(u) {
		p.mediaURLs = append(p.mediaURLs, local)
	}
	return local, nil
}

// Existing local manifests preserve nested relative playlist references. Only
// remote media/key/init resource URLs are replaced with opaque local routes.
func (p *sourceURLSegmentProxy) rewriteManifests(folder string) error {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".m3u8") {
			continue
		}
		path := filepath.Join(folder, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		lines := strings.Split(string(data), "\n")
		for i, raw := range lines {
			line := strings.TrimSpace(raw)
			if strings.HasPrefix(line, "#") {
				var replaceErr error
				lines[i] = hlsURIAttributePattern.ReplaceAllStringFunc(raw, func(attr string) string {
					if replaceErr != nil {
						return attr
					}
					u := strings.TrimSuffix(strings.TrimPrefix(attr, "URI=\""), "\"")
					if !strings.HasPrefix(u, "https://") {
						return attr
					}
					local, err := p.register(u)
					if err != nil {
						replaceErr = err
						return attr
					}
					return "URI=\"" + local + "\""
				})
				if replaceErr != nil {
					return replaceErr
				}
			} else if strings.HasPrefix(line, "https://") {
				local, err := p.register(line)
				if err != nil {
					return err
				}
				lines[i] = local
			}
		}
		if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
			return fmt.Errorf("서명 HLS 재생목록을 저장할 수 없습니다: %w", err)
		}
	}
	return nil
}

func (p *sourceURLSegmentProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, p.prefix) {
		http.NotFound(w, r)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, p.prefix)
	if extension := strings.IndexByte(id, '.'); extension >= 0 {
		id = id[:extension]
	}
	p.mu.Lock()
	target := p.resources[id]
	resourceLock := p.segmentLocks[id]
	p.requests++
	if r.Header.Get("X-CHZZK-Prefetch") == "1" {
		p.prefetchRequests++
	}
	p.mu.Unlock()
	if target == nil || resourceLock == nil {
		http.NotFound(w, r)
		return
	}
	// A speculative worker and the sequential FFmpeg consumer must not issue
	// duplicate CDN requests for the same segment.
	resourceLock.Lock()
	defer resourceLock.Unlock()
	if r.Context().Err() != nil {
		return
	}
	var cacheFile string
	if p.cacheDir != "" && sourceURLCacheableMediaURL(target) {
		cacheFile = sourceURLSegmentCacheFile(p.cacheDir, target)
		if cached, ok := readSourceURLSegmentCache(cacheFile); ok {
			p.mu.Lock()
			p.cacheHits++
			p.reusedBytes += int64(len(cached))
			delete(p.failedIDs, id)
			p.mu.Unlock()
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write(cached)
			return
		}
	}
	var refreshAttempted bool
	for attempt := 0; attempt <= sourceURLSegmentRetries; attempt++ {
		if r.Context().Err() != nil {
			return
		}
		current := p.signedTarget(target)
		status, data, err := p.fetchSegment(r.Context(), current)
		p.mu.Lock()
		p.lastStatus = status
		if attempt > 0 {
			p.retries++
		}
		p.mu.Unlock()
		if err == nil && status == http.StatusOK {
			if cacheFile != "" {
				if cacheErr := writeSourceURLSegmentCache(p.cacheDir, cacheFile, data); cacheErr != nil {
					p.mu.Lock()
					p.cacheErrors++
					p.mu.Unlock()
				} else {
					p.mu.Lock()
					p.cacheWrites++
					p.mu.Unlock()
				}
			}
			p.mu.Lock()
			delete(p.failedIDs, id)
			p.mu.Unlock()
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write(data)
			return
		}
		// Some CDN edges hang instead of returning 403 after token expiry.
		// After two transport failures, also try renewing authentication.
		authFailure := status == http.StatusUnauthorized || status == http.StatusForbidden || status == http.StatusNotFound
		transportStall := err != nil && attempt >= 1
		if (authFailure || transportStall) && !refreshAttempted {
			refreshAttempted = true
			if p.refreshSignature(r.Context(), current) == nil {
				// The same segment is retried using a fresh CDN signature.
				continue
			}
		}
		// Retry network/read timeouts and transient CDN responses. Invalid
		// tokens will only be refreshed once per segment request.
		if err == nil && status != 0 && status != http.StatusTooManyRequests && status < 500 {
			break
		}
		if attempt < sourceURLSegmentRetries {
			select {
			case <-time.After(time.Duration(attempt+1) * 300 * time.Millisecond):
			case <-r.Context().Done():
				return
			}
		}
	}
	// Even if FFmpeg skips an unavailable segment and exits successfully,
	// the final movie must not be promoted as a complete download.
	p.mu.Lock()
	if r.Header.Get("X-CHZZK-Prefetch") != "1" {
		p.failedIDs[id] = true
	}
	p.mu.Unlock()
	// Never include the segment URL, signature, or playback key in errors.
	http.Error(w, "HLS segment temporarily unavailable", http.StatusBadGateway)
}

func (p *sourceURLSegmentProxy) signedTarget(original *url.URL) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	target := *original
	query := target.Query()
	query.Del("_lsu_sa_")
	query.Set("_lsu_sa_", p.current.Query().Get("_lsu_sa_"))
	target.RawQuery = query.Encode()
	// A refreshed playlist can move to another CDN node. Preserve the
	// segment's relative location under its original playlist directory.
	previousDir := filepath.ToSlash(filepath.Dir(p.original.Path)) + "/"
	currentDir := filepath.ToSlash(filepath.Dir(p.current.Path)) + "/"
	if strings.HasPrefix(target.Path, previousDir) {
		target.Path = currentDir + strings.TrimPrefix(target.Path, previousDir)
		target.Host = p.current.Host
	}
	return target.String()
}

func (p *sourceURLSegmentProxy) refreshSignature(ctx context.Context, attemptedURL string) error {
	// Avoid global locking during an external API call. Concurrent workers
	// encountering the same expired signature share a single refresh.
	p.refreshMu.Lock()
	defer p.refreshMu.Unlock()
	attempted, err := url.Parse(attemptedURL)
	if err != nil {
		return fmt.Errorf("HLS 재생 URL을 확인하지 못했습니다")
	}
	p.mu.Lock()
	if attempted.Query().Get("_lsu_sa_") != p.current.Query().Get("_lsu_sa_") {
		p.mu.Unlock()
		return nil // Another worker already refreshed this session.
	}
	p.refreshAttempts++
	p.mu.Unlock()
	refreshCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	fresh, err := p.renew(refreshCtx)
	if err != nil || !validSourceURLHLSAddress(fresh) {
		return fmt.Errorf("새 HLS 재생 주소를 확인하지 못했습니다")
	}
	newURL, err := url.Parse(fresh)
	if err != nil || newURL.Query().Get("_lsu_sa_") == "" {
		return fmt.Errorf("새 HLS 인증 서명이 없습니다")
	}
	p.mu.Lock()
	p.current = newURL
	p.refreshes++
	p.mu.Unlock()
	return nil
}

func (p *sourceURLSegmentProxy) fetchSegment(ctx context.Context, raw string) (int, []byte, error) {
	// This per-download semaphore includes prefetch and foreground requests.
	// No job can exceed its selected 1/2/4/8 CDN connections.
	select {
	case p.downloadSlots <- struct{}{}:
		defer func() { <-p.downloadSlots }()
	case <-ctx.Done():
		return 0, nil, ctx.Err()
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, sourceURLSegmentTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(timeoutCtx, http.MethodGet, raw, nil)
	if err != nil {
		return 0, nil, fmt.Errorf("HLS 요청 형식이 잘못되었습니다")
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Referer", "https://chzzk.naver.com/")
	req.Header.Set("Origin", "https://chzzk.naver.com")
	resp, err := p.client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("HLS 세그먼트 네트워크 응답 지연")
	}
	defer resp.Body.Close()
	if !validSignedHLSResource(resp.Request.URL) {
		return 0, nil, fmt.Errorf("허용되지 않은 HLS 리다이렉트")
	}
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, nil, nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, sourceURLSegmentMaxBytes+1))
	if err != nil || len(data) > sourceURLSegmentMaxBytes {
		return resp.StatusCode, nil, fmt.Errorf("HLS 세그먼트 응답 읽기 실패")
	}
	return resp.StatusCode, data, nil
}
