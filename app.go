package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/MoonKim-isMe/chzzk-video-downloader/internal/chzzk"
	"github.com/MoonKim-isMe/chzzk-video-downloader/internal/downloader"
	"github.com/MoonKim-isMe/chzzk-video-downloader/internal/persistence"
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

	channelApplyMu sync.Mutex

	persistenceMu sync.RWMutex
	database      *persistence.Database
	databasePath  func() (string, error)

	queueMu       sync.Mutex
	downloadQueue *downloader.Queue

	settingsMu      sync.Mutex
	settingsApplyMu sync.Mutex
	settingsStore   *appsettings.Store

	shuttingDown atomic.Bool
	eventEmitter    func(downloader.DownloadTask)
	directoryPicker func(context.Context, runtime.OpenDialogOptions) (string, error)
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
	if err := a.initializePersistence(); err != nil {
		runtime.LogErrorf(ctx, "Persistence 초기화에 실패했습니다: %v", err)
	}
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
	a.channelApplyMu.Lock()
	defer a.channelApplyMu.Unlock()

	channels, err := a.channelStore.Save(channel)
	if err != nil {
		return nil, err
	}

	var saved chzzk.Channel
	for _, item := range channels {
		if strings.EqualFold(item.ChannelID, strings.TrimSpace(channel.ChannelID)) {
			saved = item
			break
		}
	}

	if database := a.persistenceDatabase(); database != nil {
		if err := database.UpsertChannel(saved); err != nil {
			a.channelStore.Remove(saved.ChannelID)
			return nil, err
		}
	}
	return channels, nil
}

func (a *App) RemoveSavedChannel(channelID string) ([]chzzk.Channel, error) {
	a.channelApplyMu.Lock()
	defer a.channelApplyMu.Unlock()

	if database := a.persistenceDatabase(); database != nil {
		if err := database.DeleteChannel(channelID); err != nil {
			return nil, err
		}
	}
	return a.channelStore.Remove(channelID), nil
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

func (a *App) SelectDownloadDirectory(currentDirectory string) (string, error) {
	defaultDirectory := strings.TrimSpace(currentDirectory)
	if defaultDirectory == "" {
		if current, err := a.GetSettings(); err == nil {
			defaultDirectory = current.DownloadDir
		}
	}

	picker := a.directoryPicker
	if picker == nil {
		picker = runtime.OpenDirectoryDialog
	}
	selected, err := picker(a.appContext(), runtime.OpenDialogOptions{
		Title:            "다운로드 폴더 선택",
		DefaultDirectory: defaultDirectory,
	})
	if err != nil {
		return "", fmt.Errorf("다운로드 폴더를 선택할 수 없습니다: %w", err)
	}
	return strings.TrimSpace(selected), nil
}

func (a *App) GetSettings() (appsettings.AppSettings, error) {
	store, err := a.ensureSettingsStore()
	if err != nil {
		return appsettings.AppSettings{}, err
	}
	return store.Get(), nil
}

func (a *App) UpdateSettings(next appsettings.AppSettings) (appsettings.AppSettings, error) {
	a.settingsApplyMu.Lock()
	defer a.settingsApplyMu.Unlock()

	store, err := a.ensureSettingsStore()
	if err != nil {
		return appsettings.AppSettings{}, err
	}

	previous := store.Get()
	updated, err := store.Update(next)
	if err != nil {
		return appsettings.AppSettings{}, err
	}

	a.queueMu.Lock()
	queue := a.downloadQueue
	a.queueMu.Unlock()
	if queue != nil {
		if err := queue.SetMaxConcurrent(updated.MaxConcurrentDownloads); err != nil {
			if _, rollbackErr := store.Update(previous); rollbackErr != nil {
				return previous, fmt.Errorf("동시 다운로드 수 적용 실패 후 설정 복구에도 실패했습니다: %v / %w", rollbackErr, err)
			}
			return previous, fmt.Errorf("동시 다운로드 수를 적용할 수 없습니다: %w", err)
		}
	}

	if database := a.persistenceDatabase(); database != nil {
		record, recordErr := appsettings.NewStorageRecord(updated)
		if recordErr != nil {
			if queue != nil {
				_ = queue.SetMaxConcurrent(previous.MaxConcurrentDownloads)
			}
			_, _ = store.Update(previous)
			return previous, recordErr
		}
		if err := database.SaveSettings(record); err != nil {
			if queue != nil {
				_ = queue.SetMaxConcurrent(previous.MaxConcurrentDownloads)
			}
			_, _ = store.Update(previous)
			return previous, err
		}
	}

	return updated, nil
}

func (a *App) StartDownload(request downloader.StartDownloadRequest) (downloader.DownloadTask, error) {
	store, err := a.ensureSettingsStore()
	if err != nil {
		return downloader.DownloadTask{}, err
	}

	request, err = downloader.ApplySettings(request, store.Get())
	if err != nil {
		return downloader.DownloadTask{}, err
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
	current := a.ensureDownloadQueue().List()
	database := a.persistenceDatabase()
	if database == nil {
		return current
	}

	persisted, err := database.ListDownloadTasks()
	if err != nil {
		runtime.LogErrorf(a.appContext(), "다운로드 이력을 조회할 수 없습니다: %v", err)
		return current
	}
	return mergeDownloadTasks(persisted, current)
}

func (a *App) CancelDownload(taskID string) bool {
	return a.ensureDownloadQueue().Cancel(taskID)
}

func (a *App) ensureDownloadQueue() *downloader.Queue {
	a.queueMu.Lock()
	defer a.queueMu.Unlock()

	if a.downloadQueue == nil {
		maxConcurrent := appsettings.DefaultMaxConcurrentDownloads
		if store, err := a.ensureSettingsStore(); err == nil {
			maxConcurrent = store.Get().MaxConcurrentDownloads
		}
		a.downloadQueue = downloader.NewQueue(
			a.appContext(),
			a.downloadManager,
			a.emitDownloadState,
			maxConcurrent,
		)
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

func (a *App) initializePersistence() error {
	pathResolver := a.databasePath
	if pathResolver == nil {
		pathResolver = persistence.DefaultPath
	}
	path, err := pathResolver()
	if err != nil {
		return err
	}
	database, err := persistence.Open(path)
	if err != nil {
		return err
	}

	fail := func(cause error) error {
		_ = database.Close()
		return cause
	}

	if err := database.RecoverInterruptedDownloads(); err != nil {
		return fail(err)
	}

	channels, err := database.ListChannels()
	if err != nil {
		return fail(err)
	}
	if err := a.channelStore.ReplaceAll(channels); err != nil {
		return fail(fmt.Errorf("저장 채널을 복원할 수 없습니다: %w", err))
	}

	downloadDir, err := downloader.DefaultOutputDir()
	if err != nil {
		return fail(err)
	}
	settingsValue := appsettings.Defaults(downloadDir)
	record, found, err := database.LoadSettings()
	if err != nil {
		return fail(err)
	}
	if found {
		settingsValue, err = record.AppSettings()
		if err != nil {
			return fail(fmt.Errorf("저장된 설정을 복원할 수 없습니다: %w", err))
		}
	} else {
		record, err = appsettings.NewStorageRecord(settingsValue)
		if err != nil {
			return fail(err)
		}
		if err := database.SaveSettings(record); err != nil {
			return fail(err)
		}
	}

	store, err := appsettings.NewStore(settingsValue)
	if err != nil {
		return fail(err)
	}

	a.settingsMu.Lock()
	a.settingsStore = store
	a.settingsMu.Unlock()

	a.persistenceMu.Lock()
	a.database = database
	a.persistenceMu.Unlock()
	return nil
}

func (a *App) persistenceDatabase() *persistence.Database {
	a.persistenceMu.RLock()
	defer a.persistenceMu.RUnlock()
	return a.database
}

func mergeDownloadTasks(
	persisted []downloader.DownloadTask,
	current []downloader.DownloadTask,
) []downloader.DownloadTask {
	currentByID := make(map[string]downloader.DownloadTask, len(current))
	for _, task := range current {
		currentByID[task.TaskID] = task
	}

	merged := make([]downloader.DownloadTask, 0, len(persisted)+len(current))
	seen := make(map[string]struct{}, len(persisted)+len(current))
	for _, task := range persisted {
		if active, ok := currentByID[task.TaskID]; ok {
			task = active
		}
		merged = append(merged, task)
		seen[task.TaskID] = struct{}{}
	}
	for _, task := range current {
		if _, ok := seen[task.TaskID]; ok {
			continue
		}
		merged = append(merged, task)
	}
	return merged
}

func (a *App) emitDownloadState(task downloader.DownloadTask) {
	if database := a.persistenceDatabase(); database != nil {
		if err := database.UpsertDownloadTask(task); err != nil {
			runtime.LogErrorf(a.appContext(), "다운로드 이력을 저장할 수 없습니다: %v", err)
		}
	}

	if a.eventEmitter != nil {
		a.eventEmitter(task)
		return
	}
	if a.shuttingDown.Load() {
		return
	}
	runtime.EventsEmit(a.appContext(), downloader.DownloadStateEvent, task)
}
