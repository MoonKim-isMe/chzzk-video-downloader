package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/MoonKim-isMe/chzzk-video-downloader/internal/downloader"
	appsettings "github.com/MoonKim-isMe/chzzk-video-downloader/internal/settings"
)

type fakeDownloadService struct {
	status   downloader.ToolchainStatus
	download func(context.Context, downloader.DownloadRequest, downloader.ProgressHandler) (downloader.DownloadResult, error)
}

func (f *fakeDownloadService) ToolchainStatus(context.Context) downloader.ToolchainStatus {
	return f.status
}

func (f *fakeDownloadService) Download(
	ctx context.Context,
	request downloader.DownloadRequest,
	handler downloader.ProgressHandler,
) (downloader.DownloadResult, error) {
	return f.download(ctx, request, handler)
}

func readyDownloadStatus() downloader.ToolchainStatus {
	return downloader.ToolchainStatus{
		YTDLP:         downloader.ToolStatus{Available: true},
		FFmpeg:        downloader.ToolStatus{Available: true},
		FFprobe:       downloader.ToolStatus{Available: true},
		DownloadReady: true,
		MergeReady:    true,
	}
}

func testStartRequest(t *testing.T) downloader.StartDownloadRequest {
	t.Helper()
	return downloader.StartDownloadRequest{
		VideoNo:     12345,
		VideoTitle:  "테스트 VOD",
		ChannelName: "테스트 채널",
		URL:         "https://chzzk.naver.com/video/12345",
		OutputDir:   t.TempDir(),
	}
}

func TestStartDownloadQueuesAndListsTasks(t *testing.T) {
	started := make(chan string, 2)
	release := make(chan struct{}, 2)

	service := &fakeDownloadService{
		status: readyDownloadStatus(),
		download: func(
			ctx context.Context,
			request downloader.DownloadRequest,
			handler downloader.ProgressHandler,
		) (downloader.DownloadResult, error) {
			started <- request.URL
			select {
			case <-release:
				return downloader.DownloadResult{
					FinalPath:    request.URL + ".mp4",
					LastProgress: downloader.DownloadProgress{Status: "completed", Percent: 100},
				}, nil
			case <-ctx.Done():
				return downloader.DownloadResult{}, ctx.Err()
			}
		},
	}
	app := NewApp()
	app.downloadManager = service
	app.eventEmitter = func(downloader.DownloadTask) {}

	first, err := app.StartDownload(testStartRequest(t))
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != downloader.TaskStatusRunning {
		t.Fatalf("expected first task running, got %s", first.Status)
	}

	secondRequest := testStartRequest(t)
	secondRequest.VideoNo = 67890
	secondRequest.URL = "https://chzzk.naver.com/video/67890"
	second, err := app.StartDownload(secondRequest)
	if err != nil {
		t.Fatal(err)
	}
	if second.Status != downloader.TaskStatusQueued {
		t.Fatalf("expected second task queued, got %s", second.Status)
	}

	tasks := app.GetDownloadTasks()
	if len(tasks) != 2 || tasks[0].TaskID != first.TaskID || tasks[1].TaskID != second.TaskID {
		t.Fatalf("unexpected task registry: %#v", tasks)
	}

	if _, err := app.StartDownload(secondRequest); err == nil {
		t.Fatal("expected duplicate VOD error")
	}

	<-started
	release <- struct{}{}
	select {
	case url := <-started:
		if url != secondRequest.URL {
			t.Fatalf("unexpected second task URL: %s", url)
		}
	case <-time.After(time.Second):
		t.Fatal("queued task did not start automatically")
	}

	release <- struct{}{}
}

func TestCancelQueuedDownloadKeepsTaskCancelled(t *testing.T) {
	started := make(chan string, 2)
	release := make(chan struct{}, 1)

	service := &fakeDownloadService{
		status: readyDownloadStatus(),
		download: func(
			ctx context.Context,
			request downloader.DownloadRequest,
			handler downloader.ProgressHandler,
		) (downloader.DownloadResult, error) {
			started <- request.URL
			select {
			case <-release:
				return downloader.DownloadResult{
					FinalPath:    request.URL + ".mp4",
					LastProgress: downloader.DownloadProgress{Status: "completed", Percent: 100},
				}, nil
			case <-ctx.Done():
				return downloader.DownloadResult{}, ctx.Err()
			}
		},
	}

	app := NewApp()
	app.downloadManager = service
	app.eventEmitter = func(downloader.DownloadTask) {}

	first, err := app.StartDownload(testStartRequest(t))
	if err != nil {
		t.Fatal(err)
	}
	<-started

	secondRequest := testStartRequest(t)
	secondRequest.VideoNo = 67890
	secondRequest.URL = "https://chzzk.naver.com/video/67890"
	second, err := app.StartDownload(secondRequest)
	if err != nil {
		t.Fatal(err)
	}
	if second.Status != downloader.TaskStatusQueued {
		t.Fatalf("expected queued task, got %s", second.Status)
	}
	if !app.CancelDownload(second.TaskID) {
		t.Fatal("expected queued cancellation")
	}

	tasks := app.GetDownloadTasks()
	if len(tasks) != 2 || tasks[1].Status != downloader.TaskStatusCancelled || tasks[1].FinishedAt == "" {
		t.Fatalf("unexpected cancelled task: %#v", tasks)
	}

	release <- struct{}{}
	deadline := time.After(100 * time.Millisecond)
	for {
		select {
		case url := <-started:
			if url == secondRequest.URL {
				t.Fatal("cancelled queued task started")
			}
		case <-deadline:
			return
		}
	}

	_ = first
}

func TestStartDownloadReportsCancellationAndStartsNext(t *testing.T) {
	cancelled := make(chan downloader.DownloadTask, 1)
	secondStarted := make(chan struct{}, 1)

	service := &fakeDownloadService{
		status: readyDownloadStatus(),
		download: func(
			ctx context.Context,
			request downloader.DownloadRequest,
			handler downloader.ProgressHandler,
		) (downloader.DownloadResult, error) {
			if request.URL == "https://chzzk.naver.com/video/67890" {
				secondStarted <- struct{}{}
				return downloader.DownloadResult{
					FinalPath:    "second.mp4",
					LastProgress: downloader.DownloadProgress{Status: "completed", Percent: 100},
				}, nil
			}
			<-ctx.Done()
			return downloader.DownloadResult{}, errors.Join(errors.New("cancelled"), ctx.Err())
		},
	}

	app := NewApp()
	app.downloadManager = service
	var mu sync.Mutex
	app.eventEmitter = func(task downloader.DownloadTask) {
		mu.Lock()
		defer mu.Unlock()
		if task.Status == downloader.TaskStatusCancelled {
			select {
			case cancelled <- task:
			default:
			}
		}
	}

	first, err := app.StartDownload(testStartRequest(t))
	if err != nil {
		t.Fatal(err)
	}
	secondRequest := testStartRequest(t)
	secondRequest.VideoNo = 67890
	secondRequest.URL = "https://chzzk.naver.com/video/67890"
	if _, err := app.StartDownload(secondRequest); err != nil {
		t.Fatal(err)
	}

	if !app.CancelDownload(first.TaskID) {
		t.Fatal("expected cancellation to succeed")
	}

	select {
	case event := <-cancelled:
		if event.TaskID != first.TaskID ||
			event.Status != downloader.TaskStatusCancelled ||
			event.FinishedAt == "" {
			t.Fatalf("unexpected immediate cancellation event: %#v", event)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("cancelled event was not emitted immediately")
	}

	current, ok := app.ensureDownloadQueue().Get(first.TaskID)
	if !ok || current.Status != downloader.TaskStatusCancelled {
		t.Fatalf("cancelled task remained running: %#v", current)
	}

	select {
	case <-secondStarted:
	case <-time.After(time.Second):
		t.Fatal("next queued task did not start")
	}
}


func TestStartDownloadSnapshotsSettingsAtEnqueueTime(t *testing.T) {
	requests := make(chan downloader.DownloadRequest, 3)
	release := make(chan struct{}, 3)

	service := &fakeDownloadService{
		status: readyDownloadStatus(),
		download: func(
			ctx context.Context,
			request downloader.DownloadRequest,
			handler downloader.ProgressHandler,
		) (downloader.DownloadResult, error) {
			requests <- request
			select {
			case <-release:
				return downloader.DownloadResult{
					FinalPath:    request.URL + "." + request.OutputFormat,
					LastProgress: downloader.DownloadProgress{Status: "completed", Percent: 100},
				}, nil
			case <-ctx.Done():
				return downloader.DownloadResult{}, ctx.Err()
			}
		},
	}

	firstDir := t.TempDir()
	secondDir := t.TempDir()
	store, err := appsettings.NewStore(appsettings.AppSettings{
		DownloadDir:            firstDir,
		Resolution:             appsettings.Resolution1080p,
		OutputFormat:           appsettings.OutputFormatMKV,
		DownloadRateLimitMBps:   12.5,
		MaxConcurrentDownloads: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	app := NewApp()
	app.downloadManager = service
	app.settingsStore = store
	app.eventEmitter = func(downloader.DownloadTask) {}

	firstRequest := testStartRequest(t)
	first, err := app.StartDownload(firstRequest)
	if err != nil {
		t.Fatal(err)
	}
	firstActual := <-requests
	if firstActual.OutputDir != firstDir ||
		firstActual.FormatSelector != "bv*[height<=1080]+ba/b[height<=1080]" ||
		firstActual.OutputFormat != "mkv" ||
		firstActual.RateLimitBytesPerSecond != 12_500_000 {
		t.Fatalf("first settings not applied: %#v", firstActual)
	}

	secondRequest := testStartRequest(t)
	secondRequest.VideoNo = 67890
	secondRequest.URL = "https://chzzk.naver.com/video/67890"
	second, err := app.StartDownload(secondRequest)
	if err != nil {
		t.Fatal(err)
	}
	if second.Status != downloader.TaskStatusQueued {
		t.Fatalf("expected queued second task, got %s", second.Status)
	}

	if _, err := app.UpdateSettings(appsettings.AppSettings{
		DownloadDir:            secondDir,
		Resolution:             appsettings.Resolution720p,
		OutputFormat:           appsettings.OutputFormatWebM,
		DownloadRateLimitMBps:   5.25,
		MaxConcurrentDownloads: 1,
	}); err != nil {
		t.Fatal(err)
	}

	release <- struct{}{}
	select {
	case secondActual := <-requests:
		if secondActual.OutputDir != firstDir ||
			secondActual.FormatSelector != "bv*[height<=1080]+ba/b[height<=1080]" ||
			secondActual.OutputFormat != "mkv" ||
			secondActual.RateLimitBytesPerSecond != 12_500_000 {
			t.Fatalf("queued task settings changed after update: %#v", secondActual)
		}
	case <-time.After(time.Second):
		t.Fatal("second task did not start")
	}

	release <- struct{}{}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		task, ok := app.ensureDownloadQueue().Get(second.TaskID)
		if ok && task.Status == downloader.TaskStatusCompleted {
			break
		}
		time.Sleep(time.Millisecond)
	}

	thirdRequest := testStartRequest(t)
	thirdRequest.VideoNo = 78901
	thirdRequest.URL = "https://chzzk.naver.com/video/78901"
	if _, err := app.StartDownload(thirdRequest); err != nil {
		t.Fatal(err)
	}
	select {
	case thirdActual := <-requests:
		if thirdActual.OutputDir != secondDir ||
			thirdActual.FormatSelector != "bv*[height<=720]+ba/b[height<=720]" ||
			thirdActual.OutputFormat != "webm" ||
			thirdActual.RateLimitBytesPerSecond != 5_250_000 {
			t.Fatalf("new settings not applied to new task: %#v", thirdActual)
		}
	case <-time.After(time.Second):
		t.Fatal("third task did not start")
	}

	release <- struct{}{}
	_ = first
}

func TestUpdateSettingsIncreasingConcurrencyStartsQueuedTask(t *testing.T) {
	started := make(chan string, 2)
	release := make(chan struct{}, 2)

	service := &fakeDownloadService{
		status: readyDownloadStatus(),
		download: func(
			ctx context.Context,
			request downloader.DownloadRequest,
			handler downloader.ProgressHandler,
		) (downloader.DownloadResult, error) {
			started <- request.URL
			select {
			case <-release:
				return downloader.DownloadResult{
					FinalPath:    request.URL + ".mp4",
					LastProgress: downloader.DownloadProgress{Status: "completed", Percent: 100},
				}, nil
			case <-ctx.Done():
				return downloader.DownloadResult{}, ctx.Err()
			}
		},
	}

	store, err := appsettings.NewStore(appsettings.Defaults(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	app := NewApp()
	app.downloadManager = service
	app.settingsStore = store
	app.eventEmitter = func(downloader.DownloadTask) {}

	if _, err := app.StartDownload(testStartRequest(t)); err != nil {
		t.Fatal(err)
	}
	<-started

	secondRequest := testStartRequest(t)
	secondRequest.VideoNo = 67890
	secondRequest.URL = "https://chzzk.naver.com/video/67890"
	second, err := app.StartDownload(secondRequest)
	if err != nil {
		t.Fatal(err)
	}
	if second.Status != downloader.TaskStatusQueued {
		t.Fatalf("expected second task queued, got %s", second.Status)
	}

	current := store.Get()
	current.MaxConcurrentDownloads = 2
	if _, err := app.UpdateSettings(current); err != nil {
		t.Fatal(err)
	}

	select {
	case url := <-started:
		if url != secondRequest.URL {
			t.Fatalf("unexpected started URL: %s", url)
		}
	case <-time.After(time.Second):
		t.Fatal("queued task did not start after increasing concurrency")
	}

	release <- struct{}{}
	release <- struct{}{}
}


func TestRunningDownloadKeepsSettingsSnapshotAfterUpdate(t *testing.T) {
	started := make(chan struct{}, 1)
	observe := make(chan struct{})
	observed := make(chan downloader.DownloadRequest, 1)
	release := make(chan struct{})

	firstDir := t.TempDir()
	secondDir := t.TempDir()

	service := &fakeDownloadService{
		status: readyDownloadStatus(),
		download: func(
			ctx context.Context,
			request downloader.DownloadRequest,
			handler downloader.ProgressHandler,
		) (downloader.DownloadResult, error) {
			started <- struct{}{}
			select {
			case <-observe:
				observed <- request
			case <-ctx.Done():
				return downloader.DownloadResult{}, ctx.Err()
			}
			select {
			case <-release:
				return downloader.DownloadResult{
					FinalPath:    request.URL + "." + request.OutputFormat,
					LastProgress: downloader.DownloadProgress{Status: "completed", Percent: 100},
				}, nil
			case <-ctx.Done():
				return downloader.DownloadResult{}, ctx.Err()
			}
		},
	}

	store, err := appsettings.NewStore(appsettings.AppSettings{
		DownloadDir:            firstDir,
		Resolution:             appsettings.Resolution1440p,
		OutputFormat:           appsettings.OutputFormatMKV,
		DownloadRateLimitMBps:   10.5,
		MaxConcurrentDownloads: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	app := NewApp()
	app.downloadManager = service
	app.settingsStore = store
	app.eventEmitter = func(downloader.DownloadTask) {}

	if _, err := app.StartDownload(testStartRequest(t)); err != nil {
		t.Fatal(err)
	}
	<-started

	if _, err := app.UpdateSettings(appsettings.AppSettings{
		DownloadDir:            secondDir,
		Resolution:             appsettings.Resolution720p,
		OutputFormat:           appsettings.OutputFormatWebM,
		DownloadRateLimitMBps:   2.25,
		MaxConcurrentDownloads: 1,
	}); err != nil {
		t.Fatal(err)
	}

	close(observe)
	select {
	case request := <-observed:
		if request.OutputDir != firstDir ||
			request.FormatSelector != "bv*[height<=1440]+ba/b[height<=1440]" ||
			request.OutputFormat != "mkv" ||
			request.RateLimitBytesPerSecond != 10_500_000 {
			t.Fatalf("running task snapshot changed: %#v", request)
		}
	case <-time.After(time.Second):
		t.Fatal("running request was not observed")
	}
	close(release)
}

func TestUpdateSettingsDecreasingConcurrencyWaitsForActiveSlots(t *testing.T) {
	started := make(chan string, 3)
	var releaseMu sync.Mutex
	releases := map[string]chan struct{}{}

	service := &fakeDownloadService{
		status: readyDownloadStatus(),
		download: func(
			ctx context.Context,
			request downloader.DownloadRequest,
			handler downloader.ProgressHandler,
		) (downloader.DownloadResult, error) {
			release := make(chan struct{})
			releaseMu.Lock()
			releases[request.URL] = release
			releaseMu.Unlock()
			started <- request.URL
			select {
			case <-release:
				return downloader.DownloadResult{
					FinalPath:    request.URL + ".mp4",
					LastProgress: downloader.DownloadProgress{Status: "completed", Percent: 100},
				}, nil
			case <-ctx.Done():
				return downloader.DownloadResult{}, ctx.Err()
			}
		},
	}

	store, err := appsettings.NewStore(appsettings.AppSettings{
		DownloadDir:            t.TempDir(),
		Resolution:             appsettings.ResolutionBest,
		OutputFormat:           appsettings.OutputFormatMP4,
		MaxConcurrentDownloads: 2,
	})
	if err != nil {
		t.Fatal(err)
	}

	app := NewApp()
	app.downloadManager = service
	app.settingsStore = store
	app.eventEmitter = func(downloader.DownloadTask) {}

	requests := make([]downloader.StartDownloadRequest, 3)
	for index := range requests {
		requests[index] = testStartRequest(t)
		requests[index].VideoNo = int64(91000 + index)
		requests[index].URL = fmt.Sprintf("https://chzzk.naver.com/video/%d", requests[index].VideoNo)
		if _, err := app.StartDownload(requests[index]); err != nil {
			t.Fatal(err)
		}
	}

	firstStarted := <-started
	secondStarted := <-started
	if firstStarted == secondStarted {
		t.Fatal("expected two distinct running tasks")
	}

	current := store.Get()
	current.MaxConcurrentDownloads = 1
	if _, err := app.UpdateSettings(current); err != nil {
		t.Fatal(err)
	}
	if app.ensureDownloadQueue().MaxConcurrent() != 1 {
		t.Fatalf("unexpected queue concurrency: %d", app.ensureDownloadQueue().MaxConcurrent())
	}

	releaseMu.Lock()
	firstRelease := releases[firstStarted]
	releaseMu.Unlock()
	close(firstRelease)

	select {
	case url := <-started:
		t.Fatalf("third task started while one existing task was still active: %s", url)
	case <-time.After(100 * time.Millisecond):
	}

	releaseMu.Lock()
	secondRelease := releases[secondStarted]
	releaseMu.Unlock()
	close(secondRelease)

	select {
	case url := <-started:
		if url != requests[2].URL {
			t.Fatalf("unexpected third task URL: %s", url)
		}
	case <-time.After(time.Second):
		t.Fatal("third task did not start after active count dropped below limit")
	}

	releaseMu.Lock()
	thirdRelease := releases[requests[2].URL]
	releaseMu.Unlock()
	if thirdRelease != nil {
		close(thirdRelease)
	}
}


func TestRecoverDownloadCleansTemporaryFilesAndQueuesRetry(t *testing.T) {
	outputDir := t.TempDir()
	store, err := appsettings.NewStore(appsettings.Defaults(outputDir))
	if err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	attempts := 0
	release := make(chan struct{}, 1)
	service := &fakeDownloadService{
		status: readyDownloadStatus(),
		download: func(
			ctx context.Context,
			request downloader.DownloadRequest,
			handler downloader.ProgressHandler,
		) (downloader.DownloadResult, error) {
			mu.Lock()
			attempts++
			attempt := attempts
			mu.Unlock()

			if attempt == 1 {
				return downloader.DownloadResult{}, &downloader.DownloadFailure{
					Kind:    downloader.DownloadFailurePartialDataConflict,
					Message: "이전 다운로드의 임시 데이터와 충돌했습니다. 임시 파일을 정리한 뒤 다시 시도해 주세요.",
				}
			}

			select {
			case <-release:
				return downloader.DownloadResult{
					FinalPath:    request.URL + ".mp4",
					LastProgress: downloader.DownloadProgress{Status: "completed", Percent: 100},
				}, nil
			case <-ctx.Done():
				return downloader.DownloadResult{}, ctx.Err()
			}
		},
	}

	app := NewApp()
	app.downloadManager = service
	app.settingsStore = store
	app.eventEmitter = func(downloader.DownloadTask) {}

	request := testStartRequest(t)
	request.OutputDir = outputDir
	failedTask, err := app.StartDownload(request)
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		current, ok := app.ensureDownloadQueue().Get(failedTask.TaskID)
		if ok && current.Status == downloader.TaskStatusFailed {
			failedTask = current
			break
		}
		time.Sleep(time.Millisecond)
	}
	if failedTask.Status != downloader.TaskStatusFailed ||
		failedTask.ErrorCode != downloader.DownloadFailurePartialDataConflict {
		t.Fatalf("expected recoverable failed task, got %#v", failedTask)
	}

	tempDir, err := downloader.TemporaryDownloadDir(outputDir, failedTask.VideoNo)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tempDir+"/stale.part", []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	retried, err := app.RecoverDownload(failedTask.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if retried.TaskID == failedTask.TaskID {
		t.Fatal("retry must create a new task")
	}
	if _, err := os.Stat(tempDir); !os.IsNotExist(err) {
		t.Fatalf("temporary directory was not removed: %v", err)
	}
	if _, ok := app.ensureDownloadQueue().Get(failedTask.TaskID); ok {
		t.Fatal("previous failed task still exists in queue")
	}

	release <- struct{}{}
}


func TestOpenDownloadLogUsesStoredFailureLog(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "failure.log")
	if err := os.WriteFile(logPath, []byte("failure"), 0o644); err != nil {
		t.Fatal(err)
	}

	service := &fakeDownloadService{
		status: readyDownloadStatus(),
		download: func(
			context.Context,
			downloader.DownloadRequest,
			downloader.ProgressHandler,
		) (downloader.DownloadResult, error) {
			return downloader.DownloadResult{LogPath: logPath}, errors.New("download failed")
		},
	}
	store, err := appsettings.NewStore(appsettings.Defaults(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}

	app := NewApp()
	app.downloadManager = service
	app.settingsStore = store
	app.eventEmitter = func(downloader.DownloadTask) {}

	var opened string
	app.fileOpener = func(path string) error {
		opened = path
		return nil
	}

	task, err := app.StartDownload(testStartRequest(t))
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		current, ok := app.ensureDownloadQueue().Get(task.TaskID)
		if ok && current.Status == downloader.TaskStatusFailed {
			task = current
			break
		}
		time.Sleep(time.Millisecond)
	}
	if task.Status != downloader.TaskStatusFailed || task.LogPath != logPath {
		t.Fatalf("unexpected failed task: %#v", task)
	}

	if err := app.OpenDownloadLog(task.TaskID); err != nil {
		t.Fatal(err)
	}
	if opened != logPath {
		t.Fatalf("unexpected opened log path: %q", opened)
	}
}
