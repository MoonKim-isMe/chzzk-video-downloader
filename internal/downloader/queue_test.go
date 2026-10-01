package downloader

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"
)

type queueTestExecutor struct {
	mu       sync.Mutex
	started  []string
	releases map[string]chan struct{}
}

func newQueueTestExecutor() *queueTestExecutor {
	return &queueTestExecutor{releases: make(map[string]chan struct{})}
}

func (e *queueTestExecutor) Download(
	ctx context.Context,
	request DownloadRequest,
	handler ProgressHandler,
) (DownloadResult, error) {
	e.mu.Lock()
	e.started = append(e.started, request.URL)
	release := make(chan struct{})
	e.releases[request.URL] = release
	e.mu.Unlock()

	handler(DownloadProgress{Status: "downloading", Percent: 50})
	select {
	case <-release:
		return DownloadResult{
			FinalPath:    request.URL + ".mp4",
			LastProgress: DownloadProgress{Status: "completed", Percent: 100},
		}, nil
	case <-ctx.Done():
		return DownloadResult{}, ctx.Err()
	}
}

func (e *queueTestExecutor) release(url string) {
	e.mu.Lock()
	release := e.releases[url]
	e.mu.Unlock()
	if release != nil {
		close(release)
	}
}

func (e *queueTestExecutor) startedCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.started)
}

func waitForStartedCount(t *testing.T, executor *queueTestExecutor, count int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if executor.startedCount() >= count {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("executor did not start %d tasks; got %d", count, executor.startedCount())
}

func waitForTaskStatus(t *testing.T, queue *Queue, taskID string, status TaskStatus) DownloadTask {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		task, ok := queue.Get(taskID)
		if ok && task.Status == status {
			return task
		}
		time.Sleep(time.Millisecond)
	}
	task, _ := queue.Get(taskID)
	t.Fatalf("task %s did not reach %s: %#v", taskID, status, task)
	return DownloadTask{}
}

func queueRequest(videoNo int64) StartDownloadRequest {
	return StartDownloadRequest{
		VideoNo:    videoNo,
		VideoTitle: "테스트 VOD",
		URL:        "https://chzzk.naver.com/video/" + strconv.FormatInt(videoNo, 10),
		OutputDir:  "downloads",
	}
}

func TestQueueRunsTasksInFIFOOrder(t *testing.T) {
	executor := newQueueTestExecutor()
	queue := NewQueue(context.Background(), executor, nil, 1)

	first, err := queue.Enqueue(queueRequest(1001))
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != TaskStatusRunning {
		t.Fatalf("expected first task running, got %s", first.Status)
	}

	second, err := queue.Enqueue(queueRequest(1002))
	if err != nil {
		t.Fatal(err)
	}
	if second.Status != TaskStatusQueued {
		t.Fatalf("expected second task queued, got %s", second.Status)
	}

	waitForStartedCount(t, executor, 1)
	if executor.startedCount() != 1 {
		t.Fatalf("expected one running executor, got %d", executor.startedCount())
	}

	executor.release(first.URL)
	waitForTaskStatus(t, queue, first.TaskID, TaskStatusCompleted)
	waitForTaskStatus(t, queue, second.TaskID, TaskStatusRunning)

	executor.release(second.URL)
	waitForTaskStatus(t, queue, second.TaskID, TaskStatusCompleted)

	tasks := queue.List()
	if len(tasks) != 2 || tasks[0].TaskID != first.TaskID || tasks[1].TaskID != second.TaskID {
		t.Fatalf("unexpected task order: %#v", tasks)
	}
}

func TestQueueRejectsDuplicateActiveOrPendingVideo(t *testing.T) {
	executor := newQueueTestExecutor()
	queue := NewQueue(context.Background(), executor, nil, 1)

	first, _ := queue.Enqueue(queueRequest(2001))
	waitForStartedCount(t, executor, 1)
	if _, err := queue.Enqueue(queueRequest(2001)); err == nil {
		t.Fatal("expected duplicate active task error")
	}

	second, _ := queue.Enqueue(queueRequest(2002))
	if _, err := queue.Enqueue(queueRequest(2002)); err == nil {
		t.Fatal("expected duplicate queued task error")
	}

	executor.release(first.URL)
	waitForTaskStatus(t, queue, second.TaskID, TaskStatusRunning)
	executor.release(second.URL)
	waitForTaskStatus(t, queue, second.TaskID, TaskStatusCompleted)

	redownload, err := queue.Enqueue(queueRequest(2001))
	if err != nil {
		t.Fatalf("completed video should be enqueueable again: %v", err)
	}
	if redownload.VideoNo != 2001 {
		t.Fatalf("unexpected redownload task: %#v", redownload)
	}
	queue.Stop()
}

func TestQueueCancelRunningTaskStartsNext(t *testing.T) {
	executor := newQueueTestExecutor()
	queue := NewQueue(context.Background(), executor, nil, 1)

	first, _ := queue.Enqueue(queueRequest(3001))
	waitForStartedCount(t, executor, 1)
	second, _ := queue.Enqueue(queueRequest(3002))

	if !queue.Cancel(first.TaskID) {
		t.Fatal("expected active task cancellation")
	}
	waitForTaskStatus(t, queue, first.TaskID, TaskStatusCancelled)
	waitForTaskStatus(t, queue, second.TaskID, TaskStatusRunning)
	queue.Stop()
}

func TestQueueStopRejectsNewTasks(t *testing.T) {
	queue := NewQueue(context.Background(), &immediateErrorExecutor{}, nil, 1)
	queue.Stop()
	if _, err := queue.Enqueue(queueRequest(4001)); err == nil {
		t.Fatal("expected stopped queue error")
	}
}

type immediateErrorExecutor struct{}

func (*immediateErrorExecutor) Download(
	context.Context,
	DownloadRequest,
	ProgressHandler,
) (DownloadResult, error) {
	return DownloadResult{}, errors.New("not used")
}
