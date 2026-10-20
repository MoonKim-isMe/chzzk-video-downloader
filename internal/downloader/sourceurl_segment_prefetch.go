package downloader

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Restrict speculative downloads to HLS media segments intersecting the
// selected time window. The signed manifests remain intact so FFmpeg can seek
// using EXTINF durations and request a pre-roll keyframe when necessary.
// Media/key requests outside the prefetched set remain available on demand.
func (p *sourceURLSegmentProxy) restrictPrefetchToSegment(folder string, segment *DownloadSegment) {
	if segment == nil || len(p.mediaURLs) == 0 {
		return
	}
	entries, err := os.ReadDir(folder)
	if err != nil {
		return
	}
	end, finite := segmentEndExclusive(segment)
	allowed := make(map[string]struct{})
	parsedMedia := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".m3u8") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(folder, entry.Name()))
		if err != nil { return }
		media, ok := segmentPrefetchURLs(string(data), float64(segment.StartSeconds), float64(end), finite)
		if !ok { return } // Keep complete prefetch if a playlist is not safely parseable.
		if media == nil { continue } // Master playlist, not media.
		parsedMedia++
		for _, address := range media {
			allowed[address] = struct{}{}
		}
	}
	if parsedMedia == 0 {
		return
	}
	filtered := make([]string, 0, len(allowed))
	for _, address := range p.mediaURLs {
		if _, ok := allowed[address]; ok {
			filtered = append(filtered, address)
		}
	}
	p.mediaURLs = filtered
}

// A nil slice means the playlist does not contain media entries (e.g. master).
// A false parse result indicates an unknown layout; do not optimize it.
func segmentPrefetchURLs(playlist string, start, end float64, finite bool) ([]string, bool) {
	var urls []string
	var elapsed float64
	var segmentDuration float64
	var expectingURL bool
	var hasDurations bool
	for _, raw := range strings.Split(playlist, "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "#EXTINF:") {
			parts := strings.SplitN(strings.TrimPrefix(line, "#EXTINF:"), ",", 2)
			value, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
			if err != nil || value <= 0 {
				return nil, false
			}
			segmentDuration = value
			expectingURL = true
			hasDurations = true
			continue
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if expectingURL {
			if !(finite && elapsed >= end) && elapsed+segmentDuration > start {
				urls = append(urls, line)
			}
			elapsed += segmentDuration
			expectingURL = false
			continue
		}
		// Nested master playlists have local playlist names, without EXTINF.
		if strings.HasSuffix(strings.ToLower(line), ".m3u8") {
			continue
		}
		// A segment URI without EXTINF means we cannot infer the timeline.
		return nil, false
	}
	if expectingURL {
		return nil, false
	}
	if !hasDurations {
		return nil, true
	}
	return urls, true
}
