package downloader

import (
	"context"
	"errors"
	"strconv"
	"strings"
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


func TestQueueRapidEnqueuePreservesFIFOOrder(t *testing.T) {
	executor := newControlledExecutor()
	queue := NewQueue(context.Background(), executor, nil, 1)

	const count = 12
	tasks := make([]DownloadTask, 0, count)
	for index := 0; index < count; index++ {
		task, err := queue.Enqueue(queueRequest(int64(8000 + index)))
		if err != nil {
			t.Fatal(err)
		}
		tasks = append(tasks, task)
	}

	waitForStartedCount(t, executor, 1)
	for index, task := range tasks {
		if index == 0 {
			waitForTaskStatus(t, queue, task.TaskID, TaskStatusRunning)
		} else {
			waitForTaskStatus(t, queue, task.TaskID, TaskStatusQueued)
		}
	}

	for index, task := range tasks {
		if index > 0 {
			waitForTaskStatus(t, queue, task.TaskID, TaskStatusRunning)
		}
		executor.release(task.URL)
		waitForTaskStatus(t, queue, task.TaskID, TaskStatusCompleted)
	}

	executor.mu.Lock()
	started := append([]string(nil), executor.started...)
	executor.mu.Unlock()
	if len(started) != count {
		t.Fatalf("expected %d started tasks, got %d", count, len(started))
	}
	for index, task := range tasks {
		if started[index] != task.URL {
			t.Fatalf("FIFO order mismatch at %d: got %s want %s", index, started[index], task.URL)
		}
	}
}

func TestQueueSetMaxConcurrentStartsAdditionalPendingTasks(t *testing.T) {
	executor := newControlledExecutor()
	queue := NewQueue(context.Background(), executor, nil, 1)

	first, _ := queue.Enqueue(queueRequest(9001))
	second, _ := queue.Enqueue(queueRequest(9002))
	third, _ := queue.Enqueue(queueRequest(9003))
	waitForStartedCount(t, executor, 1)

	if queue.MaxConcurrent() != 1 {
		t.Fatalf("unexpected initial maxConcurrent: %d", queue.MaxConcurrent())
	}
	if err := queue.SetMaxConcurrent(2); err != nil {
		t.Fatal(err)
	}
	if queue.MaxConcurrent() != 2 {
		t.Fatalf("unexpected updated maxConcurrent: %d", queue.MaxConcurrent())
	}

	waitForTaskStatus(t, queue, second.TaskID, TaskStatusRunning)
	waitForStartedCount(t, executor, 2)

	if err := queue.SetMaxConcurrent(1); err != nil {
		t.Fatal(err)
	}
	executor.release(first.URL)
	waitForTaskStatus(t, queue, first.TaskID, TaskStatusCompleted)

	time.Sleep(20 * time.Millisecond)
	if executor.hasStarted(third.URL) {
		t.Fatal("third task must stay queued while active count equals reduced limit")
	}

	executor.release(second.URL)
	waitForTaskStatus(t, queue, second.TaskID, TaskStatusCompleted)
	waitForTaskStatus(t, queue, third.TaskID, TaskStatusRunning)
	queue.Stop()
}

func TestQueueSetMaxConcurrentRejectsInvalidValue(t *testing.T) {
	queue := NewQueue(context.Background(), &immediateErrorExecutor{}, nil, 1)
	if err := queue.SetMaxConcurrent(0); err == nil {
		t.Fatal("expected invalid concurrency error")
	}
	if queue.MaxConcurrent() != 1 {
		t.Fatalf("invalid update changed maxConcurrent: %d", queue.MaxConcurrent())
	}
	queue.Stop()
	if err := queue.SetMaxConcurrent(2); err == nil {
		t.Fatal("expected stopped queue update error")
	}
}

type parsedProgressExecutor struct{}

func (*parsedProgressExecutor) Download(
	ctx context.Context,
	request DownloadRequest,
	handler ProgressHandler,
) (DownloadResult, error) {
	line := progressPrefix + "downloading\t" + strconv.FormatInt(requestProgressBytes(request.URL), 10) +
		"\t1000\tNA\t250\t2\t50.0%"
	progress, ok := parseProgressLine(line)
	if !ok {
		return DownloadResult{}, errors.New("progress parse failed")
	}
	handler(progress)

	select {
	case <-ctx.Done():
		return DownloadResult{}, ctx.Err()
	default:
	}

	return DownloadResult{
		FinalPath: request.URL + ".mp4",
		LastProgress: DownloadProgress{
			Status:          "completed",
			Percent:         100,
			DownloadedBytes: 1000,
			TotalBytes:      1000,
		},
	}, nil
}

func requestProgressBytes(url string) int64 {
	if strings.HasSuffix(url, "/10001") {
		return 111
	}
	return 222
}

func TestQueueKeepsParsedProgressIsolatedPerTask(t *testing.T) {
	var mu sync.Mutex
	events := make(map[int64][]DownloadTask)
	queue := NewQueue(context.Background(), &parsedProgressExecutor{}, func(task DownloadTask) {
		mu.Lock()
		events[task.VideoNo] = append(events[task.VideoNo], task)
		mu.Unlock()
	}, 2)

	first, err := queue.Enqueue(queueRequest(10001))
	if err != nil {
		t.Fatal(err)
	}
	second, err := queue.Enqueue(queueRequest(10002))
	if err != nil {
		t.Fatal(err)
	}

	waitForTaskStatus(t, queue, first.TaskID, TaskStatusCompleted)
	waitForTaskStatus(t, queue, second.TaskID, TaskStatusCompleted)

	mu.Lock()
	defer mu.Unlock()

	var firstSaw111, secondSaw222 bool
	for _, event := range events[10001] {
		if event.Status == TaskStatusRunning &&
			event.Progress.DownloadedBytes == 111 &&
			event.Progress.TotalBytes == 1000 &&
			event.Progress.SpeedBytesPerSecond == 250 &&
			event.Progress.ETASeconds == 2 &&
			event.Progress.Percent == 50 {
			firstSaw111 = true
		}
	}
	for _, event := range events[10002] {
		if event.Status == TaskStatusRunning &&
			event.Progress.DownloadedBytes == 222 &&
			event.Progress.TotalBytes == 1000 &&
			event.Progress.SpeedBytesPerSecond == 250 &&
			event.Progress.ETASeconds == 2 &&
			event.Progress.Percent == 50 {
			secondSaw222 = true
		}
	}
	if !firstSaw111 || !secondSaw222 {
		t.Fatalf("progress leaked or was not delivered: first=%#v second=%#v", events[10001], events[10002])
	}
}

func TestQueueEventStatusNeverRegresses(t *testing.T) {
	var mu sync.Mutex
	events := make(map[string][]TaskStatus)
	executor := newControlledExecutor()
	queue := NewQueue(context.Background(), executor, func(task DownloadTask) {
		mu.Lock()
		events[task.TaskID] = append(events[task.TaskID], task.Status)
		mu.Unlock()
	}, 1)

	first, _ := queue.Enqueue(queueRequest(11001))
	second, _ := queue.Enqueue(queueRequest(11002))
	waitForStartedCount(t, executor, 1)

	executor.release(first.URL)
	waitForTaskStatus(t, queue, first.TaskID, TaskStatusCompleted)
	waitForTaskStatus(t, queue, second.TaskID, TaskStatusRunning)
	queue.Cancel(second.TaskID)
	waitForTaskStatus(t, queue, second.TaskID, TaskStatusCancelled)

	rank := map[TaskStatus]int{
		TaskStatusQueued:    0,
		TaskStatusRunning:   1,
		TaskStatusCompleted: 2,
		TaskStatusFailed:    2,
		TaskStatusCancelled: 2,
	}

	mu.Lock()
	defer mu.Unlock()
	for taskID, statuses := range events {
		previous := -1
		for _, status := range statuses {
			current := rank[status]
			if current < previous {
				t.Fatalf("task %s status regressed: %#v", taskID, statuses)
			}
			previous = current
		}
	}
}


func TestQueueRunningCancellationIsImmediateButKeepsSchedulerSlotUntilExit(t *testing.T) {
	executor := newControlledExecutor()
	firstRequest := queueRequest(12001)
	executor.ignoreCancellation(firstRequest.URL)

	cancelledEvents := make(chan DownloadTask, 1)
	queue := NewQueue(context.Background(), executor, func(task DownloadTask) {
		if task.Status == TaskStatusCancelled {
			select {
			case cancelledEvents <- task:
			default:
			}
		}
	}, 1)

	first, err := queue.Enqueue(firstRequest)
	if err != nil {
		t.Fatal(err)
	}
	waitForStartedCount(t, executor, 1)

	second, err := queue.Enqueue(queueRequest(12002))
	if err != nil {
		t.Fatal(err)
	}
	if second.Status != TaskStatusQueued {
		t.Fatalf("expected queued second task, got %s", second.Status)
	}

	if !queue.Cancel(first.TaskID) {
		t.Fatal("expected running cancellation")
	}

	current, ok := queue.Get(first.TaskID)
	if !ok || current.Status != TaskStatusCancelled || current.FinishedAt == "" {
		t.Fatalf("running cancellation was not visible immediately: %#v", current)
	}

	select {
	case event := <-cancelledEvents:
		if event.TaskID != first.TaskID || event.Status != TaskStatusCancelled {
			t.Fatalf("unexpected immediate cancellation event: %#v", event)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("cancelled event was not emitted immediately")
	}

	time.Sleep(20 * time.Millisecond)
	if executor.hasStarted(second.URL) {
		t.Fatal("next task started before cancelled executor actually exited")
	}

	executor.release(first.URL)
	waitForTaskStatus(t, queue, second.TaskID, TaskStatusRunning)
	queue.Stop()
}

func TestQueueRemoveOnlyRemovesTerminalTasks(t *testing.T) {
	executor := newControlledExecutor()
	queue := NewQueue(context.Background(), executor, nil, 1)

	task, err := queue.Enqueue(queueRequest(13001))
	if err != nil {
		t.Fatal(err)
	}
	waitForStartedCount(t, executor, 1)
	if queue.Remove(task.TaskID) {
		t.Fatal("running task must not be removable")
	}

	executor.release(task.URL)
	waitForTaskStatus(t, queue, task.TaskID, TaskStatusCompleted)

	if !queue.Remove(task.TaskID) {
		t.Fatal("completed task should be removable")
	}
	if _, ok := queue.Get(task.TaskID); ok {
		t.Fatal("removed task still exists in registry")
	}
	if len(queue.List()) != 0 {
		t.Fatalf("removed task still exists in queue order: %#v", queue.List())
	}
}


func TestQueueStoresStructuredFailureCode(t *testing.T) {
	executor := newControlledExecutor()
	request := queueRequest(14001)
	executor.fail(request.URL, &DownloadFailure{
		Kind:    DownloadFailurePartialDataConflict,
		Message: "임시 파일 충돌",
	})

	queue := NewQueue(context.Background(), executor, nil, 1)
	task, err := queue.Enqueue(request)
	if err != nil {
		t.Fatal(err)
	}
	waitForStartedCount(t, executor, 1)
	executor.release(request.URL)

	failed := waitForTaskStatus(t, queue, task.TaskID, TaskStatusFailed)
	if failed.ErrorCode != DownloadFailurePartialDataConflict {
		t.Fatalf("unexpected error code: %q", failed.ErrorCode)
	}
	if failed.Error != "임시 파일 충돌" {
		t.Fatalf("unexpected error message: %q", failed.Error)
	}
}
