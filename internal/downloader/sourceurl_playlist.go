package downloader

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	maxSignedHLSPlaylistBytes = 4 * 1024 * 1024
	maxSignedHLSPlaylists     = 16
	maxSignedHLSDepth         = 4
)

var hlsURIAttributePattern = regexp.MustCompile(`URI="([^"]+)"`)

type signedHLSPlaylistWriter struct {
	client *http.Client
	folder string
	count  int
}

// prepareSignedHLSPlaylist creates local HLS manifests whose media segment URIs
// carry the same short-lived CDN signature as the original playlist URI. FFmpeg
// otherwise resolves relative segment URLs without the playlist's query string.
// Every generated file must be removed when the downloader finishes or fails.
func prepareSignedHLSPlaylist(ctx context.Context, client *http.Client, sourceURL, folder string) (string, error) {
	if !validSourceURLHLSAddress(sourceURL) {
		return "", fmt.Errorf("유효하지 않은 HLS 재생 주소입니다")
	}
	if err := os.MkdirAll(folder, 0o700); err != nil {
		return "", fmt.Errorf("HLS 재생목록 임시 폴더를 만들 수 없습니다: %w", err)
	}
	writer := &signedHLSPlaylistWriter{client: client, folder: folder}
	return writer.prepare(ctx, sourceURL, 0)
}

func (w *signedHLSPlaylistWriter) prepare(ctx context.Context, rawURL string, depth int) (string, error) {
	if depth > maxSignedHLSDepth || w.count >= maxSignedHLSPlaylists {
		return "", fmt.Errorf("HLS 재생목록이 지원하는 중첩 범위를 초과했습니다")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || !validSignedHLSResource(parsed) || !strings.EqualFold(filepath.Ext(parsed.Path), ".m3u8") {
		return "", fmt.Errorf("유효하지 않은 HLS 재생목록 주소입니다")
	}
	w.count++
	playlistName := fmt.Sprintf("playlist-%02d.m3u8", w.count)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("HLS 재생목록 요청을 만들 수 없습니다")
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Origin", "https://chzzk.naver.com")
	req.Header.Set("Referer", "https://chzzk.naver.com/")
	req.Header.Set("Accept", "application/vnd.apple.mpegurl, application/x-mpegURL, */*")
	response, err := w.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		// Do not expose the signed URL embedded in a net/url.Error.
		return "", fmt.Errorf("HLS 재생목록을 요청할 수 없습니다")
	}
	defer response.Body.Close()
	if !validSignedHLSResource(response.Request.URL) {
		return "", fmt.Errorf("HLS 재생목록이 허용되지 않은 서버로 이동했습니다")
	}
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HLS 재생목록 서버가 HTTP %d를 반환했습니다", response.StatusCode)
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, maxSignedHLSPlaylistBytes+1))
	if err != nil || len(content) > maxSignedHLSPlaylistBytes {
		return "", fmt.Errorf("HLS 재생목록의 내용을 읽을 수 없습니다")
	}

	base := response.Request.URL
	lines := strings.Split(strings.TrimPrefix(string(content), "\ufeff"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "#EXTM3U" {
		return "", fmt.Errorf("올바르지 않은 HLS 재생목록입니다")
	}

	for i, raw := range lines {
		line := strings.TrimSuffix(raw, "\r")
		if strings.HasPrefix(line, "#") {
			if !strings.Contains(line, `URI="`) {
				lines[i] = line
				continue
			}
			var replaceErr error
			updated := hlsURIAttributePattern.ReplaceAllStringFunc(line, func(attr string) string {
				if replaceErr != nil {
					return attr
				}
				uri := strings.TrimSuffix(strings.TrimPrefix(attr, `URI="`), `"`)
				absolute, resolveErr := signedHLSChildURL(base, uri)
				if resolveErr != nil {
					replaceErr = resolveErr
					return attr
				}
				if strings.EqualFold(filepath.Ext(absolute.Path), ".m3u8") {
					var local string
					local, replaceErr = w.prepare(ctx, absolute.String(), depth+1)
					if replaceErr != nil {
						return attr
					}
					return `URI="` + filepath.Base(local) + `"`
				}
				return `URI="` + absolute.String() + `"`
			})
			if replaceErr != nil {
				return "", replaceErr
			}
			lines[i] = updated
			continue
		}
		if strings.TrimSpace(line) == "" {
			lines[i] = line
			continue
		}
		absolute, resolveErr := signedHLSChildURL(base, strings.TrimSpace(line))
		if resolveErr != nil {
			return "", resolveErr
		}
		if strings.EqualFold(filepath.Ext(absolute.Path), ".m3u8") {
			local, nestedErr := w.prepare(ctx, absolute.String(), depth+1)
			if nestedErr != nil {
				return "", nestedErr
			}
			lines[i] = filepath.Base(local)
		} else {
			lines[i] = absolute.String()
		}
	}
	path := filepath.Join(w.folder, playlistName)
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
		return "", fmt.Errorf("HLS 재생목록 임시 파일을 저장할 수 없습니다: %w", err)
	}
	return path, nil
}

func validSignedHLSResource(target *url.URL) bool {
	if target == nil || target.Scheme != "https" || target.User != nil || target.Fragment != "" || (target.Port() != "" && target.Port() != "443") {
		return false
	}
	host := strings.ToLower(target.Hostname())
	return host == "pstatic.net" || strings.HasSuffix(host, ".pstatic.net")
}

func signedHLSChildURL(parent *url.URL, raw string) (*url.URL, error) {
	child, err := url.Parse(raw)
	if err != nil || child == nil || child.Opaque != "" {
		return nil, fmt.Errorf("잘못된 HLS 세그먼트 주소입니다")
	}
	absolute := parent.ResolveReference(child)
	if !validSignedHLSResource(absolute) {
		return nil, fmt.Errorf("허용되지 않은 HLS 세그먼트 서버입니다")
	}
	// CDN-signed playback manifests use relative TS paths. URL.ResolveReference
	// drops the parent's query string; restore only the known CDN signature.
	if absolute.Hostname() == parent.Hostname() {
		parentSignature := parent.Query().Get("_lsu_sa_")
		if parentSignature != "" && absolute.Query().Get("_lsu_sa_") == "" {
			query := absolute.Query()
			query.Set("_lsu_sa_", parentSignature)
			absolute.RawQuery = query.Encode()
		}
	}
	return absolute, nil
}
