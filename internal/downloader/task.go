package downloader

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const DownloadStateEvent = "download:state"

type TaskStatus string

const (
	TaskStatusQueued    TaskStatus = "queued"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
)

type DownloadType string

const (
	DownloadTypeVideo DownloadType = "video"
	DownloadTypeAudio DownloadType = "audio"
)

func NormalizeDownloadType(value DownloadType) DownloadType {
	if value == "" {
		return DownloadTypeVideo
	}
	return value
}

type StartDownloadRequest struct {
	DownloadType            DownloadType          `json:"downloadType,omitempty"`
	VideoNo                 int64                 `json:"videoNo"`
	VideoTitle              string                `json:"videoTitle"`
	ChannelName             string                `json:"channelName"`
	ThumbnailImageURL       string                `json:"thumbnailImageUrl"`
	URL                     string                `json:"url"`
	OutputDir               string                `json:"outputDir"`
	FormatSelector          string                `json:"formatSelector,omitempty"`
	OutputTemplate          string                `json:"outputTemplate,omitempty"`
	OutputFormat            string                `json:"outputFormat,omitempty"`
	ConcurrentFragments     int                   `json:"concurrentFragments,omitempty"`
	RateLimitBytesPerSecond int64                 `json:"rateLimitBytesPerSecond,omitempty"`
	Authentication          AuthenticationOptions `json:"authentication,omitempty"`
}

type DownloadTask struct {
	DownloadType      DownloadType     `json:"downloadType"`
	TaskID            string           `json:"taskId"`
	VideoNo           int64            `json:"videoNo"`
	VideoTitle        string           `json:"videoTitle"`
	ChannelName       string           `json:"channelName"`
	ThumbnailImageURL string           `json:"thumbnailImageUrl"`
	URL               string           `json:"url"`
	OutputDir         string           `json:"outputDir"`
	Status            TaskStatus       `json:"status"`
	Progress          DownloadProgress `json:"progress"`
	FinalPath         string              `json:"finalPath,omitempty"`
	LogPath           string              `json:"logPath,omitempty"`
	Error             string              `json:"error,omitempty"`
	ErrorCode         DownloadFailureKind `json:"errorCode,omitempty"`
	QueuedAt          string              `json:"queuedAt"`
	StartedAt         string           `json:"startedAt,omitempty"`
	FinishedAt        string           `json:"finishedAt,omitempty"`
}

func (r StartDownloadRequest) Validate() error {
	if kind := NormalizeDownloadType(r.DownloadType); kind != DownloadTypeVideo && kind != DownloadTypeAudio {
		return fmt.Errorf("지원하지 않는 다운로드 유형입니다: %s", kind)
	}
	if r.VideoNo <= 0 {
		return fmt.Errorf("유효한 VOD 번호가 필요합니다")
	}
	if strings.TrimSpace(r.VideoTitle) == "" {
		return fmt.Errorf("VOD 제목이 필요합니다")
	}
	if _, err := normalizeVideoURL(r.URL); err != nil {
		return err
	}
	if _, err := normalizeOutputDir(r.OutputDir); err != nil {
		return err
	}
	return nil
}

func (r StartDownloadRequest) DownloadRequest() DownloadRequest {
	return DownloadRequest{
		DownloadType: NormalizeDownloadType(r.DownloadType),
		URL:            r.URL,
		OutputDir:      r.OutputDir,
		FormatSelector: r.FormatSelector,
		OutputTemplate:      r.OutputTemplate,
		OutputFormat:            r.OutputFormat,
		ConcurrentFragments:     r.ConcurrentFragments,
		RateLimitBytesPerSecond: r.RateLimitBytesPerSecond,
		Authentication:          r.Authentication,
	}
}

func DefaultOutputDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("사용자 홈 디렉터리를 확인할 수 없습니다: %w", err)
	}
	if strings.TrimSpace(home) == "" {
		return "", fmt.Errorf("사용자 홈 디렉터리를 확인할 수 없습니다")
	}
	return filepath.Join(home, "Downloads", "CHZZK Video Downloader"), nil
}
