package downloader

import (
	"context"
	"io"
	"net/http"
	"time"
)

// startParallelPrefetch uses the existing authenticated loopback proxy and
// persistent segment cache. FFmpeg still reads its playlist sequentially,
// while up to 2/4/8 CDN requests run in parallel for this download only.
//
// A bounded job channel and fixed worker pool avoid loading the entire VOD
// into memory. Cache misses from FFmpeg use the same download semaphore.
func (p *sourceURLSegmentProxy) startParallelPrefetch(parent context.Context, acceleration int) {
	if acceleration <= 1 || p.cacheDir == "" || len(p.mediaURLs) == 0 {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	p.prefetchCancel = cancel
	jobs := make(chan string, acceleration*2)

	p.prefetchWorkers.Add(acceleration + 1)
	go func() {
		defer p.prefetchWorkers.Done()
		defer close(jobs)
		for _, address := range p.mediaURLs {
			select {
			case jobs <- address:
			case <-ctx.Done():
				return
			}
		}
	}()

	// The HTTP worker is deliberately separate from the CDN client because
	// the latter enforces allowed pstatic.net redirects and may use an
	// injected transport in tests.
	client := &http.Client{Timeout: 2 * time.Minute}
	for i := 0; i < acceleration; i++ {
		go func() {
			defer p.prefetchWorkers.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case address, ok := <-jobs:
					if !ok {
						return
					}
					request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
					if err != nil {
						continue
					}
					request.Header.Set("X-CHZZK-Prefetch", "1")
					response, err := client.Do(request)
					if err != nil {
						continue
					}
					// Body is already a bounded segment from the proxy.
					_, _ = io.Copy(io.Discard, response.Body)
					response.Body.Close()
				}
			}
		}()
	}
}
