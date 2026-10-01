package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/MoonKim-isMe/chzzk-video-downloader/internal/downloader"
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
		if event.Error == "" {
			t.Fatal("expected cancellation error message")
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled event timeout")
	}

	select {
	case <-secondStarted:
	case <-time.After(time.Second):
		t.Fatal("next queued task did not start")
	}
}
