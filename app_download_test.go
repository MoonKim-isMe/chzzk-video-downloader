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

func TestStartDownloadEmitsProgressAndCompletion(t *testing.T) {
	var mu sync.Mutex
	var events []downloader.DownloadTask
	done := make(chan struct{})

	service := &fakeDownloadService{
		status: readyDownloadStatus(),
		download: func(
			ctx context.Context,
			request downloader.DownloadRequest,
			handler downloader.ProgressHandler,
		) (downloader.DownloadResult, error) {
			progress := downloader.DownloadProgress{Status: "downloading", Percent: 50}
			handler(progress)
			return downloader.DownloadResult{
				FinalPath:    `C:\\Video\\done.mp4`,
				LastProgress: downloader.DownloadProgress{Status: "completed", Percent: 100},
			}, nil
		},
	}
	app := NewApp()
	app.downloadManager = service
	app.eventEmitter = func(task downloader.DownloadTask) {
		mu.Lock()
		events = append(events, task)
		if task.Status == downloader.TaskStatusCompleted {
			select {
			case <-done:
			default:
				close(done)
			}
		}
		mu.Unlock()
	}

	task, err := app.StartDownload(testStartRequest(t))
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != downloader.TaskStatusRunning {
		t.Fatalf("unexpected initial status: %s", task.Status)
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("completion event timeout")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(events) < 3 {
		t.Fatalf("expected initial, progress and completion events: %#v", events)
	}
	last := events[len(events)-1]
	if last.Status != downloader.TaskStatusCompleted || last.FinalPath == "" || last.Progress.Percent != 100 {
		t.Fatalf("unexpected completion event: %#v", last)
	}
}

func TestStartDownloadRejectsConcurrentTaskAndCanCancel(t *testing.T) {
	started := make(chan struct{})
	finished := make(chan struct{})

	service := &fakeDownloadService{
		status: readyDownloadStatus(),
		download: func(
			ctx context.Context,
			request downloader.DownloadRequest,
			handler downloader.ProgressHandler,
		) (downloader.DownloadResult, error) {
			close(started)
			<-ctx.Done()
			close(finished)
			return downloader.DownloadResult{}, ctx.Err()
		},
	}
	app := NewApp()
	app.downloadManager = service
	app.eventEmitter = func(downloader.DownloadTask) {}

	task, err := app.StartDownload(testStartRequest(t))
	if err != nil {
		t.Fatal(err)
	}
	<-started

	second := testStartRequest(t)
	second.VideoNo = 67890
	second.URL = "https://chzzk.naver.com/video/67890"
	if _, err := app.StartDownload(second); err == nil {
		t.Fatal("expected concurrent download error")
	}
	if !app.CancelDownload(task.TaskID) {
		t.Fatal("expected cancellation to succeed")
	}

	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("cancellation timeout")
	}
}

func TestStartDownloadReportsCancellation(t *testing.T) {
	cancelled := make(chan downloader.DownloadTask, 1)
	service := &fakeDownloadService{
		status: readyDownloadStatus(),
		download: func(
			ctx context.Context,
			request downloader.DownloadRequest,
			handler downloader.ProgressHandler,
		) (downloader.DownloadResult, error) {
			<-ctx.Done()
			return downloader.DownloadResult{}, errors.Join(errors.New("cancelled"), ctx.Err())
		},
	}

	app := NewApp()
	app.downloadManager = service
	app.eventEmitter = func(task downloader.DownloadTask) {
		if task.Status == downloader.TaskStatusCancelled {
			cancelled <- task
		}
	}

	task, err := app.StartDownload(testStartRequest(t))
	if err != nil {
		t.Fatal(err)
	}
	if !app.CancelDownload(task.TaskID) {
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
}
