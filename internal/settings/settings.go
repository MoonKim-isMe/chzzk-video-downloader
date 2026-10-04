package settings

import (
	"fmt"
	"math"
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

type DownloadAcceleration string

const (
	DownloadAccelerationStable   DownloadAcceleration = "stable"
	DownloadAccelerationStandard DownloadAcceleration = "standard"
	DownloadAccelerationFast     DownloadAcceleration = "fast"
	DownloadAccelerationUltra    DownloadAcceleration = "ultra"
)

const DefaultDownloadAcceleration = DownloadAccelerationStandard

type ThemeMode string

const (
	ThemeLight ThemeMode = "light"
	ThemeDark  ThemeMode = "dark"
)

const (
	DefaultMaxConcurrentDownloads = 3
	MaxConcurrentDownloads        = 8
	DefaultDownloadRateLimitMBps  = 0.0
)

type AppSettings struct {
	DownloadDir            string               `json:"downloadDir"`
	Resolution             Resolution           `json:"resolution"`
	OutputFormat           OutputFormat         `json:"outputFormat"`
	DownloadAcceleration   DownloadAcceleration `json:"downloadAcceleration"`
	DownloadRateLimitMBps   float64              `json:"downloadRateLimitMBps"`
	MaxConcurrentDownloads int                  `json:"maxConcurrentDownloads"`
	Theme                  ThemeMode            `json:"theme"`
}

func Defaults(downloadDir string) AppSettings {
	return AppSettings{
		DownloadDir:            filepath.Clean(strings.TrimSpace(downloadDir)),
		Resolution:             ResolutionBest,
		OutputFormat:           OutputFormatMP4,
		DownloadAcceleration:   DefaultDownloadAcceleration,
		DownloadRateLimitMBps:   DefaultDownloadRateLimitMBps,
		MaxConcurrentDownloads: DefaultMaxConcurrentDownloads,
		Theme:                  ThemeDark,
	}
}

func Normalize(value AppSettings) AppSettings {
	value.DownloadDir = filepath.Clean(strings.TrimSpace(value.DownloadDir))
	value.Resolution = Resolution(strings.ToLower(strings.TrimSpace(string(value.Resolution))))
	value.OutputFormat = OutputFormat(strings.ToLower(strings.TrimSpace(string(value.OutputFormat))))
	value.DownloadAcceleration = DownloadAcceleration(strings.ToLower(strings.TrimSpace(string(value.DownloadAcceleration))))
	if value.DownloadAcceleration == "" {
		value.DownloadAcceleration = DefaultDownloadAcceleration
	}
	value.Theme = ThemeMode(strings.ToLower(strings.TrimSpace(string(value.Theme))))
	if value.Theme == "" {
		value.Theme = ThemeDark
	}
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

	switch value.DownloadAcceleration {
	case DownloadAccelerationStable, DownloadAccelerationStandard, DownloadAccelerationFast, DownloadAccelerationUltra:
	default:
		return fmt.Errorf("지원하지 않는 다운로드 가속 설정입니다: %s", value.DownloadAcceleration)
	}

	if math.IsNaN(value.DownloadRateLimitMBps) || math.IsInf(value.DownloadRateLimitMBps, 0) || value.DownloadRateLimitMBps < 0 {
		return fmt.Errorf("다운로드 속도 제한은 0 이상의 유효한 MB/s 값이어야 합니다")
	}

	if value.MaxConcurrentDownloads < 1 || value.MaxConcurrentDownloads > MaxConcurrentDownloads {
		return fmt.Errorf("동시 다운로드 수는 1~%d 사이여야 합니다", MaxConcurrentDownloads)
	}

	switch value.Theme {
	case ThemeLight, ThemeDark:
	default:
		return fmt.Errorf("지원하지 않는 테마입니다: %s", value.Theme)
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
