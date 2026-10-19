package downloader

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const sourceURLResumeDirectoryName = ".chzzk-resume"

// CDN signatures are per-session credentials. They must never become part of
// the cache identity, otherwise a fresh session cannot reuse stored segments.
var sourceURLCacheSignature = regexp.MustCompile(`(_lsu_sa_=)[^&\s"']+`)

// sourceURLSegmentCacheDir uses the exact downloaded media manifest as the
// cache identity, ignoring only the short-lived CDN signature. A different
// quality, video, or segment manifest cannot reuse an unrelated recording.
func sourceURLSegmentCacheDir(outputDir string, videoNo int64, height int, manifestDir string) (string, error) {
	if videoNo <= 0 || height <= 0 {
		return "", fmt.Errorf("이어받기 영상 정보를 확인할 수 없습니다")
	}
	normalized, err := normalizeOutputDir(outputDir)
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(manifestDir)
	if err != nil {
		return "", fmt.Errorf("HLS 이어받기 목록을 읽을 수 없습니다: %w", err)
	}
	hasher := sha256.New()
	var manifests int
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".m3u8") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(manifestDir, entry.Name()))
		if err != nil {
			return "", fmt.Errorf("HLS 이어받기 목록을 읽을 수 없습니다: %w", err)
		}
		manifests++
		_, _ = hasher.Write([]byte(entry.Name() + "\n"))
		_, _ = hasher.Write(sourceURLCacheSignature.ReplaceAll(data, []byte("1")))
		_, _ = hasher.Write([]byte("\n"))
	}
	if manifests == 0 {
		return "", fmt.Errorf("HLS 이어받기 재생목록이 없습니다")
	}
	fingerprint := hex.EncodeToString(hasher.Sum(nil))[:24]
	return filepath.Join(normalized, sourceURLResumeDirectoryName,
		strconv.FormatInt(videoNo, 10), strconv.Itoa(height), fingerprint), nil
}

func sourceURLCacheableMediaURL(u *url.URL) bool {
	if !validSignedHLSResource(u) {
		return false
	}
	switch strings.ToLower(filepath.Ext(u.Path)) {
	case ".ts", ".m4s", ".m4a", ".mp4", ".aac":
		return true
	default:
		// Never persist encryption keys or secrets across download sessions.
		return false
	}
}

func sourceURLSegmentCacheFile(folder string, u *url.URL) string {
	identity := u.EscapedPath()
	params := u.Query()
	params.Del("_lsu_sa_")
	if query := params.Encode(); query != "" {
		identity += "?" + query
	}
	digest := sha256.Sum256([]byte(identity))
	return filepath.Join(folder, hex.EncodeToString(digest[:])+".segment")
}

// Only complete HTTP 200 responses are cached. Writing a temporary file and
// renaming it ensures canceled, interrupted, or crashed requests cannot leave
// partial media that would be mistaken for a reusable segment.
func writeSourceURLSegmentCache(folder, path string, content []byte) error {
	file, err := os.CreateTemp(folder, ".segment-*.partial")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(content); err != nil {
		file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	// Another concurrent request may already have cached the same segment.
	// Incomplete or invalid older entries must not block replacement.
	if _, valid := readSourceURLSegmentCache(path); valid {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(file.Name(), path)
}

func readSourceURLSegmentCache(path string) ([]byte, bool) {
	file, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > sourceURLSegmentMaxBytes {
		return nil, false
	}
	data := make([]byte, info.Size())
	if _, err = file.ReadAt(data, 0); err != nil {
		return nil, false
	}
	return data, true
}
