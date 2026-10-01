package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/MoonKim-isMe/chzzk-video-downloader/internal/chzzk"
	"github.com/MoonKim-isMe/chzzk-video-downloader/internal/downloader"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	appName    = "CHZZK Video Downloader"
	appVersion = "0.1.0"
)

type downloadService interface {
	ToolchainStatus(context.Context) downloader.ToolchainStatus
	Download(context.Context, downloader.DownloadRequest, downloader.ProgressHandler) (downloader.DownloadResult, error)
}

type activeDownload struct {
	taskID  string
	videoNo int64
	cancel  context.CancelFunc
}

type App struct {
	ctx             context.Context
	chzzkClient     *chzzk.Client
	channelStore    *chzzk.Store
	downloadManager downloadService

	downloadMu      sync.Mutex
	activeDownload  *activeDownload
	downloadCounter atomic.Uint64
	shuttingDown    atomic.Bool
	eventEmitter    func(downloader.DownloadTask)
}

type AppInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Runtime string `json:"runtime"`
}

func NewApp() *App {
	return &App{
		chzzkClient:     chzzk.NewClient(),
		channelStore:    chzzk.NewStore(),
		downloadManager: downloader.NewManager(),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) shutdown(context.Context) {
	a.shuttingDown.Store(true)
	a.cancelActiveDownload()
}

func (a *App) appContext() context.Context {
	if a.ctx != nil {
		return a.ctx
	}
	return context.Background()
}

func (a *App) GetAppInfo() AppInfo {
	return AppInfo{
		Name:    appName,
		Version: appVersion,
		Runtime: "Wails v2",
	}
}

func (a *App) SearchChannels(keyword string, offset, size int) (chzzk.ChannelSearchResult, error) {
	return a.chzzkClient.SearchChannels(a.appContext(), keyword, offset, size)
}

func (a *App) ResolveChannelURL(rawURL string) (chzzk.Channel, error) {
	channelID, _, err := chzzk.ParseChannelURL(rawURL)
	if err != nil {
		return chzzk.Channel{}, err
	}
	return a.chzzkClient.GetChannel(a.appContext(), channelID)
}

func (a *App) GetSavedChannels() []chzzk.Channel {
	return a.channelStore.List()
}

func (a *App) SaveChannel(channel chzzk.Channel) ([]chzzk.Channel, error) {
	return a.channelStore.Save(channel)
}

func (a *App) RemoveSavedChannel(channelID string) []chzzk.Channel {
	return a.channelStore.Remove(channelID)
}

func (a *App) GetChannelVideos(channelID string, page, size int) (chzzk.VideoListResult, error) {
	return a.chzzkClient.GetChannelVideos(a.appContext(), channelID, page, size)
}

func (a *App) GetDownloadToolchainStatus() downloader.ToolchainStatus {
	return a.downloadManager.ToolchainStatus(a.appContext())
}

func (a *App) GetDefaultDownloadDir() (string, error) {
	return downloader.DefaultOutputDir()
}

func (a *App) StartDownload(request downloader.StartDownloadRequest) (downloader.DownloadTask, error) {
	if request.OutputDir == "" {
		outputDir, err := downloader.DefaultOutputDir()
		if err != nil {
			return downloader.DownloadTask{}, err
		}
		request.OutputDir = outputDir
	}
	if err := request.Validate(); err != nil {
		return downloader.DownloadTask{}, err
	}

	toolchain := a.downloadManager.ToolchainStatus(a.appContext())
	if !toolchain.DownloadReady {
		return downloader.DownloadTask{}, fmt.Errorf("yt-dlp 실행 환경이 준비되지 않았습니다")
	}
	if !toolchain.MergeReady {
		return downloader.DownloadTask{}, fmt.Errorf("ffmpeg/ffprobe 실행 환경이 준비되지 않았습니다")
	}

	a.downloadMu.Lock()
	if a.activeDownload != nil {
		a.downloadMu.Unlock()
		return downloader.DownloadTask{}, fmt.Errorf("이미 진행 중인 다운로드가 있습니다")
	}

	taskID := fmt.Sprintf("download-%d-%d", request.VideoNo, a.downloadCounter.Add(1))
	downloadCtx, cancel := context.WithCancel(a.appContext())
	a.activeDownload = &activeDownload{
		taskID:  taskID,
		videoNo: request.VideoNo,
		cancel:  cancel,
	}
	a.downloadMu.Unlock()

	task := downloader.DownloadTask{
		TaskID:            taskID,
		VideoNo:           request.VideoNo,
		VideoTitle:        request.VideoTitle,
		ChannelName:       request.ChannelName,
		ThumbnailImageURL: request.ThumbnailImageURL,
		URL:               request.URL,
		OutputDir:         request.OutputDir,
		Status:            downloader.TaskStatusRunning,
		StartedAt:         time.Now().UTC().Format(time.RFC3339Nano),
	}
	a.emitDownloadState(task)

	go a.runDownload(downloadCtx, task, request.DownloadRequest())
	return task, nil
}

func (a *App) CancelDownload(taskID string) bool {
	a.downloadMu.Lock()
	active := a.activeDownload
	if active == nil || active.taskID != taskID {
		a.downloadMu.Unlock()
		return false
	}
	cancel := active.cancel
	a.downloadMu.Unlock()

	cancel()
	return true
}

func (a *App) runDownload(ctx context.Context, task downloader.DownloadTask, request downloader.DownloadRequest) {
	result, err := a.downloadManager.Download(ctx, request, func(progress downloader.DownloadProgress) {
		task.Progress = progress
		a.emitDownloadState(task)
	})

	task.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			task.Status = downloader.TaskStatusCancelled
		} else {
			task.Status = downloader.TaskStatusFailed
		}
		task.Error = err.Error()
	} else {
		task.Status = downloader.TaskStatusCompleted
		task.Progress = result.LastProgress
		task.FinalPath = result.FinalPath
	}
	a.clearActiveDownload(task.TaskID)
	a.emitDownloadState(task)
}

func (a *App) emitDownloadState(task downloader.DownloadTask) {
	if a.eventEmitter != nil {
		a.eventEmitter(task)
		return
	}
	if a.shuttingDown.Load() {
		return
	}
	runtime.EventsEmit(a.appContext(), downloader.DownloadStateEvent, task)
}

func (a *App) clearActiveDownload(taskID string) {
	a.downloadMu.Lock()
	defer a.downloadMu.Unlock()
	if a.activeDownload != nil && a.activeDownload.taskID == taskID {
		a.activeDownload = nil
	}
}

func (a *App) cancelActiveDownload() {
	a.downloadMu.Lock()
	active := a.activeDownload
	a.downloadMu.Unlock()
	if active != nil {
		active.cancel()
	}
}
