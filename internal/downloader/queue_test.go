package downloader

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"
)

type controlledExecutor struct {
	mu            sync.Mutex
	started       []string
	releases      map[string]chan struct{}
	failures      map[string]error
	ignoreContext map[string]bool
}

func newControlledExecutor() *controlledExecutor {
	return &controlledExecutor{
		releases:      make(map[string]chan struct{}),
		failures:      make(map[string]error),
		ignoreContext: make(map[string]bool),
	}
}

func (e *controlledExecutor) Download(
	ctx context.Context,
	request DownloadRequest,
	handler ProgressHandler,
) (DownloadResult, error) {
	e.mu.Lock()
	release := make(chan struct{})
	e.releases[request.URL] = release
	e.started = append(e.started, request.URL)
	failure := e.failures[request.URL]
	ignoreContext := e.ignoreContext[request.URL]
	e.mu.Unlock()

	handler(DownloadProgress{Status: "downloading", Percent: 50})

	if ignoreContext {
		<-release
	} else {
		select {
		case <-release:
		case <-ctx.Done():
			return DownloadResult{}, ctx.Err()
		}
	}

	if failure != nil {
		return DownloadResult{}, failure
	}
	return DownloadResult{
		FinalPath:    request.URL + ".mp4",
		LastProgress: DownloadProgress{Status: "completed", Percent: 100},
	}, nil
}

func (e *controlledExecutor) release(url string) {
	e.mu.Lock()
	release := e.releases[url]
	e.mu.Unlock()
	if release != nil {
		close(release)
	}
}

func (e *controlledExecutor) fail(url string, err error) {
	e.mu.Lock()
	e.failures[url] = err
	e.mu.Unlock()
}

func (e *controlledExecutor) ignoreCancellation(url string) {
	e.mu.Lock()
	e.ignoreContext[url] = true
	e.mu.Unlock()
}

func (e *controlledExecutor) startedCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.started)
}

func (e *controlledExecutor) hasStarted(url string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, startedURL := range e.started {
		if startedURL == url {
			return true
		}
	}
	return false
}

func waitForStartedCount(t *testing.T, executor *controlledExecutor, count int) {
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
	executor := newControlledExecutor()
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
	executor := newControlledExecutor()
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

	if _, err := queue.Enqueue(queueRequest(2001)); err != nil {
		t.Fatalf("completed video should be enqueueable again: %v", err)
	}
	queue.Stop()
}

func TestQueueCancelQueuedTaskRemovesItFromPending(t *testing.T) {
	executor := newControlledExecutor()
	queue := NewQueue(context.Background(), executor, nil, 1)

	first, _ := queue.Enqueue(queueRequest(3001))
	waitForStartedCount(t, executor, 1)
	second, _ := queue.Enqueue(queueRequest(3002))
	third, _ := queue.Enqueue(queueRequest(3003))

	if !queue.Cancel(second.TaskID) {
		t.Fatal("expected queued cancellation")
	}
	cancelled := waitForTaskStatus(t, queue, second.TaskID, TaskStatusCancelled)
	if cancelled.FinishedAt == "" {
		t.Fatal("queued cancellation should set finishedAt")
	}
	if executor.hasStarted(second.URL) {
		t.Fatal("cancelled queued task must not start")
	}
	if queue.Cancel(second.TaskID) {
		t.Fatal("terminal task cancellation should return false")
	}

	executor.release(first.URL)
	waitForTaskStatus(t, queue, third.TaskID, TaskStatusRunning)
	if executor.hasStarted(second.URL) {
		t.Fatal("cancelled task started after previous task completed")
	}
	queue.Stop()
}

func TestQueueFailureStartsNextTask(t *testing.T) {
	executor := newControlledExecutor()
	firstRequest := queueRequest(4001)
	executor.fail(firstRequest.URL, errors.New("download failed"))
	queue := NewQueue(context.Background(), executor, nil, 1)

	first, _ := queue.Enqueue(firstRequest)
	waitForStartedCount(t, executor, 1)
	second, _ := queue.Enqueue(queueRequest(4002))

	executor.release(first.URL)
	failed := waitForTaskStatus(t, queue, first.TaskID, TaskStatusFailed)
	if failed.Error == "" {
		t.Fatal("failed task should retain error")
	}
	waitForTaskStatus(t, queue, second.TaskID, TaskStatusRunning)
	queue.Stop()
}

func TestQueueRunningCancellationWinsCompletionRace(t *testing.T) {
	executor := newControlledExecutor()
	request := queueRequest(5001)
	executor.ignoreCancellation(request.URL)
	queue := NewQueue(context.Background(), executor, nil, 1)

	task, _ := queue.Enqueue(request)
	waitForStartedCount(t, executor, 1)

	if !queue.Cancel(task.TaskID) {
		t.Fatal("expected running cancellation")
	}
	executor.release(task.URL)

	cancelled := waitForTaskStatus(t, queue, task.TaskID, TaskStatusCancelled)
	if cancelled.FinalPath != "" {
		t.Fatalf("cancelled task should not expose completed file: %#v", cancelled)
	}
	if queue.Cancel(task.TaskID) {
		t.Fatal("completed cancellation transition must be idempotent")
	}
}

func TestQueueCancelRunningTaskStartsNext(t *testing.T) {
	executor := newControlledExecutor()
	queue := NewQueue(context.Background(), executor, nil, 1)

	first, _ := queue.Enqueue(queueRequest(6001))
	waitForStartedCount(t, executor, 1)
	second, _ := queue.Enqueue(queueRequest(6002))

	if !queue.Cancel(first.TaskID) {
		t.Fatal("expected active task cancellation")
	}
	waitForTaskStatus(t, queue, first.TaskID, TaskStatusCancelled)
	waitForTaskStatus(t, queue, second.TaskID, TaskStatusRunning)
	queue.Stop()
}

func TestQueueStopCancelsQueuedAndRunningTasks(t *testing.T) {
	executor := newControlledExecutor()
	queue := NewQueue(context.Background(), executor, nil, 1)

	first, _ := queue.Enqueue(queueRequest(7001))
	waitForStartedCount(t, executor, 1)
	second, _ := queue.Enqueue(queueRequest(7002))

	queue.Stop()

	waitForTaskStatus(t, queue, first.TaskID, TaskStatusCancelled)
	queued := waitForTaskStatus(t, queue, second.TaskID, TaskStatusCancelled)
	if queued.FinishedAt == "" {
		t.Fatal("stopped queued task should have finishedAt")
	}
	if executor.hasStarted(second.URL) {
		t.Fatal("stopped queued task must not start")
	}
	if _, err := queue.Enqueue(queueRequest(7003)); err == nil {
		t.Fatal("expected stopped queue error")
	}
}
