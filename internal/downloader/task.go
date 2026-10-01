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

type StartDownloadRequest struct {
	VideoNo           int64  `json:"videoNo"`
	VideoTitle        string `json:"videoTitle"`
	ChannelName       string `json:"channelName"`
	ThumbnailImageURL string `json:"thumbnailImageUrl"`
	URL               string `json:"url"`
	OutputDir         string `json:"outputDir"`
	FormatSelector    string `json:"formatSelector,omitempty"`
	OutputTemplate    string `json:"outputTemplate,omitempty"`
}

type DownloadTask struct {
	TaskID            string           `json:"taskId"`
	VideoNo           int64            `json:"videoNo"`
	VideoTitle        string           `json:"videoTitle"`
	ChannelName       string           `json:"channelName"`
	ThumbnailImageURL string           `json:"thumbnailImageUrl"`
	URL               string           `json:"url"`
	OutputDir         string           `json:"outputDir"`
	Status            TaskStatus       `json:"status"`
	Progress          DownloadProgress `json:"progress"`
	FinalPath         string           `json:"finalPath,omitempty"`
	Error             string           `json:"error,omitempty"`
	QueuedAt          string           `json:"queuedAt"`
	StartedAt         string           `json:"startedAt,omitempty"`
	FinishedAt        string           `json:"finishedAt,omitempty"`
}

func (r StartDownloadRequest) Validate() error {
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
		URL:            r.URL,
		OutputDir:      r.OutputDir,
		FormatSelector: r.FormatSelector,
		OutputTemplate: r.OutputTemplate,
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
