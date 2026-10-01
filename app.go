package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/MoonKim-isMe/chzzk-video-downloader/internal/chzzk"
	"github.com/MoonKim-isMe/chzzk-video-downloader/internal/downloader"
	appsettings "github.com/MoonKim-isMe/chzzk-video-downloader/internal/settings"
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

type App struct {
	ctx             context.Context
	chzzkClient     *chzzk.Client
	channelStore    *chzzk.Store
	downloadManager downloadService

	queueMu       sync.Mutex
	downloadQueue *downloader.Queue

	settingsMu    sync.Mutex
	settingsStore *appsettings.Store

	shuttingDown atomic.Bool
	eventEmitter func(downloader.DownloadTask)
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
	a.ensureDownloadQueue()
}

func (a *App) shutdown(context.Context) {
	a.shuttingDown.Store(true)
	a.queueMu.Lock()
	queue := a.downloadQueue
	a.queueMu.Unlock()
	if queue != nil {
		queue.Stop()
	}
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

func (a *App) GetSettings() (appsettings.AppSettings, error) {
	store, err := a.ensureSettingsStore()
	if err != nil {
		return appsettings.AppSettings{}, err
	}
	return store.Get(), nil
}

func (a *App) UpdateSettings(next appsettings.AppSettings) (appsettings.AppSettings, error) {
	store, err := a.ensureSettingsStore()
	if err != nil {
		return appsettings.AppSettings{}, err
	}
	return store.Update(next)
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

	return a.ensureDownloadQueue().Enqueue(request)
}

func (a *App) GetDownloadTasks() []downloader.DownloadTask {
	return a.ensureDownloadQueue().List()
}

func (a *App) CancelDownload(taskID string) bool {
	return a.ensureDownloadQueue().Cancel(taskID)
}

func (a *App) ensureDownloadQueue() *downloader.Queue {
	a.queueMu.Lock()
	defer a.queueMu.Unlock()

	if a.downloadQueue == nil {
		a.downloadQueue = downloader.NewQueue(a.appContext(), a.downloadManager, a.emitDownloadState, 1)
	}
	return a.downloadQueue
}

func (a *App) ensureSettingsStore() (*appsettings.Store, error) {
	a.settingsMu.Lock()
	defer a.settingsMu.Unlock()

	if a.settingsStore != nil {
		return a.settingsStore, nil
	}

	downloadDir, err := downloader.DefaultOutputDir()
	if err != nil {
		return nil, err
	}
	store, err := appsettings.NewStore(appsettings.Defaults(downloadDir))
	if err != nil {
		return nil, err
	}
	a.settingsStore = store
	return store, nil
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
