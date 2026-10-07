package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/MoonKim-isMe/chzzk-video-downloader/internal/chzzk"
	"github.com/MoonKim-isMe/chzzk-video-downloader/internal/downloader"
	"github.com/MoonKim-isMe/chzzk-video-downloader/internal/persistence"
	appsettings "github.com/MoonKim-isMe/chzzk-video-downloader/internal/settings"
	"github.com/MoonKim-isMe/chzzk-video-downloader/internal/videorepair"
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

	authenticationMu      sync.Mutex
	authenticationApplyMu sync.Mutex
	authenticationStore   *appsettings.AuthenticationStore

	shuttingDown atomic.Bool
	eventEmitter    func(downloader.DownloadTask)
	directoryPicker func(context.Context, runtime.OpenDialogOptions) (string, error)
	filePicker      func(context.Context, runtime.OpenDialogOptions) (string, error)
	folderOpener    func(string) error
	fileOpener      func(string) error
	fileRevealer    func(string) error

	videoAccessChecker func(context.Context, int64, string) error
	videoFileProbe     func(context.Context, string, string) (videorepair.FileInfo, error)
	quickVideoInspector func(context.Context, string, string, string, videorepair.ProgressHandler) (videorepair.InspectionResult, error)
	deepVideoInspector  func(context.Context, string, string, string, videorepair.ProgressHandler) (videorepair.InspectionResult, error)
	videoRepairer       func(context.Context, string, string, videorepair.RepairPlan, videorepair.RepairProgressHandler) (videorepair.RepairResult, error)

	videoOperationMu          sync.Mutex
	videoInspectionCancel     context.CancelFunc
	videoRepairCancel         context.CancelFunc
	inspectionProgressEmitter func(videorepair.InspectionProgress)
	repairProgressEmitter     func(videorepair.RepairProgress)
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
	if a.videoAccessChecker == nil {
		a.videoAccessChecker = func(ctx context.Context, videoNo int64, cookiesFilePath string) error {
			_, err := a.chzzkClient.GetVideoWithCookiesFile(ctx, videoNo, cookiesFilePath)
			return err
		}
	}
	if err := a.initializePersistence(); err != nil {
		runtime.LogErrorf(ctx, "Persistence 초기화에 실패했습니다: %v", err)
	}
	a.ensureDownloadQueue()
}

func (a *App) shutdown(context.Context) {
	a.shuttingDown.Store(true)
	a.CancelVideoInspection()
	a.CancelVideoRepair()
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
	cookiesFilePath, err := a.activeAuthenticationCookiesFile()
	if err != nil {
		return chzzk.ChannelSearchResult{}, err
	}
	return a.chzzkClient.SearchChannelsWithCookiesFile(
		a.appContext(),
		keyword,
		offset,
		size,
		cookiesFilePath,
	)
}

func (a *App) ResolveVideoURL(rawURL string) (chzzk.Video, error) {
	videoNo, _, err := chzzk.ParseVideoURL(rawURL)
	if err != nil {
		return chzzk.Video{}, err
	}
	cookiesFilePath, err := a.activeAuthenticationCookiesFile()
	if err != nil {
		return chzzk.Video{}, err
	}
	return a.chzzkClient.GetVideoWithCookiesFile(a.appContext(), videoNo, cookiesFilePath)
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
	cookiesFilePath, err := a.activeAuthenticationCookiesFile()
	if err != nil {
		return chzzk.VideoListResult{}, err
	}
	return a.chzzkClient.GetChannelVideosWithCookiesFile(
		a.appContext(),
		channelID,
		page,
		size,
		cookiesFilePath,
	)
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
	defaultDirectory = existingDirectoryForDialog(defaultDirectory)

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

func existingDirectoryForDialog(directory string) string {
	candidate := strings.TrimSpace(directory)
	if candidate == "" {
		return ""
	}

	candidate = filepath.Clean(candidate)
	if candidate == "." {
		return ""
	}

	for {
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return candidate
		}

		parent := filepath.Dir(candidate)
		if parent == candidate {
			return ""
		}
		candidate = parent
	}
}

func (a *App) SelectVideoRepairFile(currentFile string) (videorepair.FileInfo, error) {
	currentFile = strings.TrimSpace(currentFile)
	defaultDirectory := ""
	if currentFile != "" {
		defaultDirectory = filepath.Dir(filepath.Clean(currentFile))
		if defaultDirectory == "." {
			defaultDirectory = ""
		}
	}

	picker := a.filePicker
	if picker == nil {
		picker = runtime.OpenFileDialog
	}
	selected, err := picker(a.appContext(), runtime.OpenDialogOptions{
		Title:            "검사할 동영상 선택",
		DefaultDirectory: defaultDirectory,
		Filters: []runtime.FileFilter{
			{
				DisplayName: "동영상 파일",
				Pattern:     videorepair.FileDialogPattern(),
			},
		},
	})
	if err != nil {
		return videorepair.FileInfo{}, fmt.Errorf("동영상 파일을 선택할 수 없습니다: %w", err)
	}
	selected = strings.TrimSpace(selected)
	if selected == "" {
		return videorepair.FileInfo{}, nil
	}

	ffprobePath := ""
	toolchain := a.downloadManager.ToolchainStatus(a.appContext())
	if toolchain.FFprobe.Available {
		ffprobePath = toolchain.FFprobe.Path
	}

	probe := a.videoFileProbe
	if probe == nil {
		probe = videorepair.ProbeFile
	}
	info, err := probe(a.appContext(), ffprobePath, selected)
	if err != nil {
		return videorepair.FileInfo{}, fmt.Errorf("동영상 파일을 확인할 수 없습니다: %w", err)
	}
	return info, nil
}

func (a *App) StartQuickVideoInspection(path string) (videorepair.InspectionResult, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return videorepair.InspectionResult{}, fmt.Errorf("검사할 동영상 파일이 필요합니다")
	}

	toolchain := a.downloadManager.ToolchainStatus(a.appContext())
	if !toolchain.FFprobe.Available {
		return videorepair.InspectionResult{}, fmt.Errorf("빠른 검사에 필요한 영상 정보 확인 기능이 준비되지 않았습니다")
	}
	if !toolchain.FFmpeg.Available {
		return videorepair.InspectionResult{}, fmt.Errorf("빠른 검사에 필요한 영상 확인 기능이 준비되지 않았습니다")
	}

	a.videoOperationMu.Lock()
	if a.videoInspectionCancel != nil || a.videoRepairCancel != nil {
		a.videoOperationMu.Unlock()
		return videorepair.InspectionResult{}, fmt.Errorf("다른 동영상 검사 또는 복구가 이미 진행 중입니다")
	}
	inspectionContext, cancel := context.WithCancel(a.appContext())
	a.videoInspectionCancel = cancel
	a.videoOperationMu.Unlock()

	defer func() {
		cancel()
		a.videoOperationMu.Lock()
		a.videoInspectionCancel = nil
		a.videoOperationMu.Unlock()
	}()

	inspector := a.quickVideoInspector
	if inspector == nil {
		inspector = videorepair.QuickInspect
	}

	result, err := inspector(
		inspectionContext,
		toolchain.FFprobe.Path,
		toolchain.FFmpeg.Path,
		path,
		a.emitVideoInspectionProgress,
	)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(inspectionContext.Err(), context.Canceled) {
			return videorepair.InspectionResult{}, fmt.Errorf("빠른 검사가 취소되었습니다")
		}
		return videorepair.InspectionResult{}, fmt.Errorf("빠른 검사를 완료할 수 없습니다: %w", err)
	}
	return result, nil
}

func (a *App) StartDeepVideoInspection(path string) (videorepair.InspectionResult, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return videorepair.InspectionResult{}, fmt.Errorf("검사할 동영상 파일이 필요합니다")
	}

	toolchain := a.downloadManager.ToolchainStatus(a.appContext())
	if !toolchain.FFprobe.Available {
		return videorepair.InspectionResult{}, fmt.Errorf("정밀 검사에 필요한 영상 정보 확인 기능이 준비되지 않았습니다")
	}
	if !toolchain.FFmpeg.Available {
		return videorepair.InspectionResult{}, fmt.Errorf("정밀 검사에 필요한 영상 확인 기능이 준비되지 않았습니다")
	}

	a.videoOperationMu.Lock()
	if a.videoInspectionCancel != nil || a.videoRepairCancel != nil {
		a.videoOperationMu.Unlock()
		return videorepair.InspectionResult{}, fmt.Errorf("다른 동영상 검사 또는 복구가 이미 진행 중입니다")
	}
	inspectionContext, cancel := context.WithCancel(a.appContext())
	a.videoInspectionCancel = cancel
	a.videoOperationMu.Unlock()

	defer func() {
		cancel()
		a.videoOperationMu.Lock()
		a.videoInspectionCancel = nil
		a.videoOperationMu.Unlock()
	}()

	inspector := a.deepVideoInspector
	if inspector == nil {
		inspector = videorepair.DeepInspect
	}

	result, err := inspector(
		inspectionContext,
		toolchain.FFprobe.Path,
		toolchain.FFmpeg.Path,
		path,
		a.emitVideoInspectionProgress,
	)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(inspectionContext.Err(), context.Canceled) {
			return videorepair.InspectionResult{}, fmt.Errorf("정밀 검사가 취소되었습니다")
		}
		return videorepair.InspectionResult{}, fmt.Errorf("정밀 검사를 완료할 수 없습니다: %w", err)
	}
	return result, nil
}

func (a *App) CancelVideoInspection() bool {
	a.videoOperationMu.Lock()
	cancel := a.videoInspectionCancel
	a.videoOperationMu.Unlock()
	if cancel == nil {
		return false
	}
	cancel()
	return true
}

func (a *App) emitVideoInspectionProgress(progress videorepair.InspectionProgress) {
	if a.inspectionProgressEmitter != nil {
		a.inspectionProgressEmitter(progress)
		return
	}
	if a.shuttingDown.Load() {
		return
	}
	runtime.EventsEmit(a.appContext(), videorepair.InspectionProgressEvent, progress)
}

func (a *App) CreateVideoRepairPlan(result videorepair.InspectionResult) (videorepair.RepairPlan, error) {
	plan, err := videorepair.BuildRepairPlan(result)
	if err != nil {
		return videorepair.RepairPlan{}, fmt.Errorf("복구 계획을 만들 수 없습니다: %w", err)
	}
	return plan, nil
}

func (a *App) CreateVideoCompatibilityPlan(result videorepair.InspectionResult) (videorepair.RepairPlan, error) {
	plan, err := videorepair.BuildCompatibilityRepairPlan(result)
	if err != nil {
		return videorepair.RepairPlan{}, fmt.Errorf("편집 호환성 복구 계획을 만들 수 없습니다: %w", err)
	}
	return plan, nil
}

func (a *App) StartVideoRepair(plan videorepair.RepairPlan) (videorepair.RepairResult, error) {
	toolchain := a.downloadManager.ToolchainStatus(a.appContext())
	if !toolchain.FFprobe.Available {
		return videorepair.RepairResult{}, fmt.Errorf("복구 결과를 확인하는 기능이 준비되지 않았습니다")
	}
	if !toolchain.FFmpeg.Available {
		return videorepair.RepairResult{}, fmt.Errorf("동영상 복구 기능이 준비되지 않았습니다")
	}

	a.videoOperationMu.Lock()
	if a.videoInspectionCancel != nil || a.videoRepairCancel != nil {
		a.videoOperationMu.Unlock()
		return videorepair.RepairResult{}, fmt.Errorf("다른 동영상 검사 또는 복구가 이미 진행 중입니다")
	}
	repairContext, cancel := context.WithCancel(a.appContext())
	a.videoRepairCancel = cancel
	a.videoOperationMu.Unlock()

	defer func() {
		cancel()
		a.videoOperationMu.Lock()
		a.videoRepairCancel = nil
		a.videoOperationMu.Unlock()
	}()

	repairer := a.videoRepairer
	if repairer == nil {
		repairer = videorepair.Repair
	}

	result, err := repairer(
		repairContext,
		toolchain.FFprobe.Path,
		toolchain.FFmpeg.Path,
		plan,
		a.emitVideoRepairProgress,
	)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(repairContext.Err(), context.Canceled) {
			return videorepair.RepairResult{}, fmt.Errorf("동영상 복구가 취소되었습니다")
		}
		return videorepair.RepairResult{}, fmt.Errorf("동영상 복구를 완료할 수 없습니다: %w", err)
	}
	return result, nil
}

func (a *App) CancelVideoRepair() bool {
	a.videoOperationMu.Lock()
	cancel := a.videoRepairCancel
	a.videoOperationMu.Unlock()
	if cancel == nil {
		return false
	}
	cancel()
	return true
}

func (a *App) emitVideoRepairProgress(progress videorepair.RepairProgress) {
	if a.repairProgressEmitter != nil {
		a.repairProgressEmitter(progress)
		return
	}
	if a.shuttingDown.Load() {
		return
	}
	runtime.EventsEmit(a.appContext(), videorepair.RepairProgressEvent, progress)
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

func (a *App) GetAuthenticationSettings() (appsettings.AuthenticationSettings, error) {
	store, err := a.ensureAuthenticationStore()
	if err != nil {
		return appsettings.AuthenticationSettings{}, err
	}
	return store.Get(), nil
}

func (a *App) UpdateAuthenticationSettings(next appsettings.AuthenticationSettings) (appsettings.AuthenticationSettings, error) {
	a.authenticationApplyMu.Lock()
	defer a.authenticationApplyMu.Unlock()

	store, err := a.ensureAuthenticationStore()
	if err != nil {
		return appsettings.AuthenticationSettings{}, err
	}

	previous := store.Get()
	updated, err := store.Update(next)
	if err != nil {
		return appsettings.AuthenticationSettings{}, err
	}

	if database := a.persistenceDatabase(); database != nil {
		if err := database.SaveAuthenticationSettings(updated); err != nil {
			_, _ = store.Update(previous)
			return previous, err
		}
	}

	return updated, nil
}

func (a *App) SelectAuthenticationCookiesFile(currentFile string) (string, error) {
	currentFile = strings.TrimSpace(currentFile)
	defaultDirectory := ""
	if currentFile != "" {
		defaultDirectory = filepath.Dir(filepath.Clean(currentFile))
		if defaultDirectory == "." {
			defaultDirectory = ""
		}
	}

	picker := a.filePicker
	if picker == nil {
		picker = runtime.OpenFileDialog
	}
	selected, err := picker(a.appContext(), runtime.OpenDialogOptions{
		Title:            "쿠키 파일 선택",
		DefaultDirectory: defaultDirectory,
	})
	if err != nil {
		return "", fmt.Errorf("쿠키 파일을 선택할 수 없습니다: %w", err)
	}
	return strings.TrimSpace(selected), nil
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

	authenticationStore, err := a.ensureAuthenticationStore()
	if err != nil {
		return downloader.DownloadTask{}, err
	}
	request, err = downloader.ApplyAuthenticationSettings(request, authenticationStore.Get())
	if err != nil {
		return downloader.DownloadTask{}, err
	}

	if err := request.Validate(); err != nil {
		return downloader.DownloadTask{}, err
	}
	if err := a.preflightDownloadVideoAccess(
		request.VideoNo,
		request.Authentication.CookiesFilePath,
	); err != nil {
		return downloader.DownloadTask{}, err
	}

	return a.ensureDownloadQueue().Enqueue(request)
}

func (a *App) preflightDownloadVideoAccess(videoNo int64, cookiesFilePath string) error {
	if a.videoAccessChecker == nil {
		return nil
	}

	err := a.videoAccessChecker(a.appContext(), videoNo, cookiesFilePath)
	if err == nil {
		return nil
	}
	if chzzk.IsAuthenticationRequired(err) {
		return downloader.NewAuthenticationRequiredFailure(err)
	}

	// The CHZZK API preflight is only used to intercept explicit authentication
	// failures. Transient API failures must not block the existing yt-dlp path.
	return nil
}

func (a *App) GetDownloadTasks() []downloader.DownloadTask {
	current := a.ensureDownloadQueue().List()
	database := a.persistenceDatabase()
	if database == nil {
		return visibleDownloadTasks(current)
	}

	persisted, err := database.ListDownloadTasks()
	if err != nil {
		runtime.LogErrorf(a.appContext(), "다운로드 이력을 조회할 수 없습니다: %v", err)
		return visibleDownloadTasks(current)
	}
	return visibleDownloadTasks(mergeDownloadTasks(persisted, current))
}

func (a *App) CancelDownload(taskID string) bool {
	return a.ensureDownloadQueue().Cancel(strings.TrimSpace(taskID))
}

func (a *App) RecoverDownload(taskID string) (downloader.DownloadTask, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return downloader.DownloadTask{}, fmt.Errorf("복구할 다운로드 작업 ID가 필요합니다")
	}

	task, found := a.findDownloadTask(taskID)
	if !found {
		return downloader.DownloadTask{}, fmt.Errorf("복구할 다운로드 작업을 찾을 수 없습니다")
	}
	if task.Status != downloader.TaskStatusFailed {
		return downloader.DownloadTask{}, fmt.Errorf("실패한 다운로드만 복구할 수 있습니다")
	}
	if task.ErrorCode != downloader.DownloadFailurePartialDataConflict {
		return downloader.DownloadTask{}, fmt.Errorf("임시 파일 정리로 복구할 수 있는 오류가 아닙니다")
	}

	if err := downloader.CleanupTemporaryDownload(task.OutputDir, task.VideoNo); err != nil {
		return downloader.DownloadTask{}, err
	}

	retried, err := a.StartDownload(downloader.StartDownloadRequest{
		VideoNo:           task.VideoNo,
		VideoTitle:        task.VideoTitle,
		ChannelName:       task.ChannelName,
		ThumbnailImageURL: task.ThumbnailImageURL,
		URL:               task.URL,
		OutputDir:         task.OutputDir,
	})
	if err != nil {
		return downloader.DownloadTask{}, fmt.Errorf("임시 파일은 정리했지만 다운로드를 다시 시작할 수 없습니다: %w", err)
	}

	if err := a.DeleteDownloadTask(taskID); err != nil {
		runtime.LogErrorf(a.appContext(), "복구한 이전 다운로드 이력을 삭제할 수 없습니다: %v", err)
	}
	return retried, nil
}

func (a *App) DeleteDownloadTask(taskID string) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("삭제할 다운로드 작업 ID가 필요합니다")
	}

	task, found := a.findDownloadTask(taskID)
	if !found {
		return fmt.Errorf("삭제할 다운로드 작업을 찾을 수 없습니다")
	}
	if task.Status == downloader.TaskStatusQueued || task.Status == downloader.TaskStatusRunning {
		return fmt.Errorf("진행 중인 다운로드는 삭제할 수 없습니다")
	}

	if database := a.persistenceDatabase(); database != nil {
		if err := database.DeleteDownloadTask(taskID); err != nil {
			return err
		}
	}
	a.ensureDownloadQueue().Remove(taskID)
	return nil
}

func (a *App) OpenDownloadLog(taskID string) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("다운로드 작업 ID가 필요합니다")
	}

	task, found := a.findDownloadTask(taskID)
	if !found {
		return fmt.Errorf("다운로드 작업을 찾을 수 없습니다")
	}

	logPath := strings.TrimSpace(task.LogPath)
	if logPath == "" {
		return fmt.Errorf("저장된 오류 로그가 없습니다")
	}
	info, err := os.Stat(logPath)
	if err != nil {
		return fmt.Errorf("오류 로그 파일을 열 수 없습니다: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("오류 로그 경로가 파일이 아닙니다")
	}

	opener := a.fileOpener
	if opener == nil {
		opener = openFile
	}
	if err := opener(logPath); err != nil {
		return fmt.Errorf("오류 로그 파일을 열 수 없습니다: %w", err)
	}
	return nil
}

func (a *App) OpenDownloadFolder(taskID string) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("다운로드 작업 ID가 필요합니다")
	}

	task, found := a.findDownloadTask(taskID)
	if !found {
		return fmt.Errorf("다운로드 작업을 찾을 수 없습니다")
	}

	directory := strings.TrimSpace(task.OutputDir)
	if finalPath := strings.TrimSpace(task.FinalPath); finalPath != "" {
		directory = filepath.Dir(finalPath)
	}
	if directory == "" || directory == "." {
		return fmt.Errorf("다운로드 폴더를 확인할 수 없습니다")
	}
	if info, err := os.Stat(directory); err != nil {
		return fmt.Errorf("다운로드 폴더를 열 수 없습니다: %w", err)
	} else if !info.IsDir() {
		return fmt.Errorf("다운로드 경로가 폴더가 아닙니다")
	}

	opener := a.folderOpener
	if opener == nil {
		opener = openFolder
	}
	if err := opener(directory); err != nil {
		return fmt.Errorf("다운로드 폴더를 열 수 없습니다: %w", err)
	}
	return nil
}

func (a *App) RevealVideoRepairFile(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("표시할 동영상 파일 경로가 필요합니다")
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("동영상 파일 위치를 열 수 없습니다: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("선택한 경로가 동영상 파일이 아닙니다")
	}

	revealer := a.fileRevealer
	if revealer == nil {
		revealer = revealFile
	}
	if err := revealer(path); err != nil {
		return fmt.Errorf("동영상 파일 위치를 열 수 없습니다: %w", err)
	}
	return nil
}

func (a *App) findDownloadTask(taskID string) (downloader.DownloadTask, bool) {
	if task, ok := a.ensureDownloadQueue().Get(taskID); ok {
		return task, true
	}
	if database := a.persistenceDatabase(); database != nil {
		tasks, err := database.ListDownloadTasks()
		if err == nil {
			for _, task := range tasks {
				if task.TaskID == taskID {
					return task, true
				}
			}
		}
	}
	return downloader.DownloadTask{}, false
}

func visibleDownloadTasks(tasks []downloader.DownloadTask) []downloader.DownloadTask {
	visible := make([]downloader.DownloadTask, 0, len(tasks))
	for _, task := range tasks {
		if task.Status == downloader.TaskStatusCancelled {
			continue
		}
		visible = append(visible, task)
	}
	return visible
}

func openFolder(directory string) error {
	var command *exec.Cmd
	switch goruntime.GOOS {
	case "windows":
		command = exec.Command("explorer.exe", directory)
	case "darwin":
		command = exec.Command("open", directory)
	default:
		command = exec.Command("xdg-open", directory)
	}
	return command.Start()
}

func openFile(path string) error {
	var command *exec.Cmd
	switch goruntime.GOOS {
	case "windows":
		command = exec.Command("notepad.exe", path)
	case "darwin":
		command = exec.Command("open", path)
	default:
		command = exec.Command("xdg-open", path)
	}
	return command.Start()
}

func revealFile(path string) error {
	var command *exec.Cmd
	switch goruntime.GOOS {
	case "windows":
		command = exec.Command("explorer.exe", "/select,", path)
	case "darwin":
		command = exec.Command("open", "-R", path)
	default:
		command = exec.Command("xdg-open", filepath.Dir(path))
	}
	return command.Start()
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

func (a *App) activeAuthenticationCookiesFile() (string, error) {
	store, err := a.ensureAuthenticationStore()
	if err != nil {
		return "", err
	}

	settings := store.Get()
	if !settings.Enabled {
		return "", nil
	}
	if err := appsettings.ValidateAuthentication(settings); err != nil {
		return "", err
	}
	return settings.CookiesFilePath, nil
}

func (a *App) ensureAuthenticationStore() (*appsettings.AuthenticationStore, error) {
	a.authenticationMu.Lock()
	defer a.authenticationMu.Unlock()

	if a.authenticationStore != nil {
		return a.authenticationStore, nil
	}

	store, err := appsettings.NewAuthenticationStore(appsettings.AuthenticationDefaults())
	if err != nil {
		return nil, err
	}
	a.authenticationStore = store
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

	authenticationValue, found, err := database.LoadAuthenticationSettings()
	if err != nil {
		return fail(err)
	}
	if !found {
		authenticationValue = appsettings.AuthenticationDefaults()
		if err := database.SaveAuthenticationSettings(authenticationValue); err != nil {
			return fail(err)
		}
	}
	authenticationStore, err := appsettings.NewAuthenticationStore(authenticationValue)
	if err != nil {
		return fail(err)
	}

	a.settingsMu.Lock()
	a.settingsStore = store
	a.settingsMu.Unlock()

	a.authenticationMu.Lock()
	a.authenticationStore = authenticationStore
	a.authenticationMu.Unlock()

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
		if task.Status == downloader.TaskStatusCancelled {
			if err := database.DeleteDownloadTask(task.TaskID); err != nil {
				runtime.LogErrorf(a.appContext(), "취소한 다운로드 이력을 삭제할 수 없습니다: %v", err)
			}
		} else if err := database.UpsertDownloadTask(task); err != nil {
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
