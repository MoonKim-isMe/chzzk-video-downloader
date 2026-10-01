package settings

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
)

type Resolution string

const (
	ResolutionBest  Resolution = "best"
	Resolution2160p Resolution = "2160p"
	Resolution1440p Resolution = "1440p"
	Resolution1080p Resolution = "1080p"
	Resolution720p  Resolution = "720p"
)

type OutputFormat string

const (
	OutputFormatMP4  OutputFormat = "mp4"
	OutputFormatMKV  OutputFormat = "mkv"
	OutputFormatWebM OutputFormat = "webm"
)

const (
	DefaultMaxConcurrentDownloads = 1
	MaxConcurrentDownloads        = 8
)

type AppSettings struct {
	DownloadDir            string       `json:"downloadDir"`
	Resolution             Resolution   `json:"resolution"`
	OutputFormat           OutputFormat `json:"outputFormat"`
	MaxConcurrentDownloads int          `json:"maxConcurrentDownloads"`
}

func Defaults(downloadDir string) AppSettings {
	return AppSettings{
		DownloadDir:            filepath.Clean(strings.TrimSpace(downloadDir)),
		Resolution:             ResolutionBest,
		OutputFormat:           OutputFormatMP4,
		MaxConcurrentDownloads: DefaultMaxConcurrentDownloads,
	}
}

func Normalize(value AppSettings) AppSettings {
	value.DownloadDir = filepath.Clean(strings.TrimSpace(value.DownloadDir))
	value.Resolution = Resolution(strings.ToLower(strings.TrimSpace(string(value.Resolution))))
	value.OutputFormat = OutputFormat(strings.ToLower(strings.TrimSpace(string(value.OutputFormat))))
	return value
}

func Validate(value AppSettings) error {
	value = Normalize(value)
	if value.DownloadDir == "" || value.DownloadDir == "." {
		return fmt.Errorf("다운로드 경로가 필요합니다")
	}
	if strings.ContainsRune(value.DownloadDir, '\x00') {
		return fmt.Errorf("다운로드 경로에 사용할 수 없는 문자가 포함되어 있습니다")
	}

	switch value.Resolution {
	case ResolutionBest, Resolution2160p, Resolution1440p, Resolution1080p, Resolution720p:
	default:
		return fmt.Errorf("지원하지 않는 해상도입니다: %s", value.Resolution)
	}

	switch value.OutputFormat {
	case OutputFormatMP4, OutputFormatMKV, OutputFormatWebM:
	default:
		return fmt.Errorf("지원하지 않는 출력 포맷입니다: %s", value.OutputFormat)
	}

	if value.MaxConcurrentDownloads < 1 || value.MaxConcurrentDownloads > MaxConcurrentDownloads {
		return fmt.Errorf("동시 다운로드 수는 1~%d 사이여야 합니다", MaxConcurrentDownloads)
	}
	return nil
}

type Store struct {
	mu    sync.RWMutex
	value AppSettings
}

func NewStore(defaults AppSettings) (*Store, error) {
	defaults = Normalize(defaults)
	if err := Validate(defaults); err != nil {
		return nil, err
	}
	return &Store{value: defaults}, nil
}

func (s *Store) Get() AppSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.value
}

func (s *Store) Update(next AppSettings) (AppSettings, error) {
	next = Normalize(next)
	if err := Validate(next); err != nil {
		return AppSettings{}, err
	}

	s.mu.Lock()
	s.value = next
	s.mu.Unlock()
	return next, nil
}
