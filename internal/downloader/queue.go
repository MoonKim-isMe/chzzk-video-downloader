package downloader

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type DownloadExecutor interface {
	Download(context.Context, DownloadRequest, ProgressHandler) (DownloadResult, error)
}

type TaskStateHandler func(DownloadTask)

type Queue struct {
	parent        context.Context
	executor      DownloadExecutor
	onState       TaskStateHandler
	maxConcurrent int

	mu       sync.Mutex
	tasks    map[string]DownloadTask
	requests map[string]DownloadRequest
	order    []string
	pending  []string
	active   map[string]context.CancelFunc
	counter  atomic.Uint64
	stopped  bool
}

func NewQueue(parent context.Context, executor DownloadExecutor, onState TaskStateHandler, maxConcurrent int) *Queue {
	if parent == nil {
		parent = context.Background()
	}
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}
	return &Queue{
		parent:        parent,
		executor:      executor,
		onState:       onState,
		maxConcurrent: maxConcurrent,
		tasks:         make(map[string]DownloadTask),
		requests:      make(map[string]DownloadRequest),
		active:        make(map[string]context.CancelFunc),
	}
}

func (q *Queue) Enqueue(request StartDownloadRequest) (DownloadTask, error) {
	if err := request.Validate(); err != nil {
		return DownloadTask{}, err
	}

	q.mu.Lock()
	if q.stopped {
		q.mu.Unlock()
		return DownloadTask{}, fmt.Errorf("다운로드 Queue가 종료되었습니다")
	}
	for _, taskID := range q.order {
		task := q.tasks[taskID]
		if task.VideoNo == request.VideoNo &&
			(task.Status == TaskStatusQueued || task.Status == TaskStatusRunning) {
			q.mu.Unlock()
			return DownloadTask{}, fmt.Errorf("이미 대기 또는 진행 중인 VOD입니다")
		}
	}

	taskID := fmt.Sprintf("download-%d-%d", request.VideoNo, q.counter.Add(1))
	task := DownloadTask{
		TaskID:            taskID,
		VideoNo:           request.VideoNo,
		VideoTitle:        request.VideoTitle,
		ChannelName:       request.ChannelName,
		ThumbnailImageURL: request.ThumbnailImageURL,
		URL:               request.URL,
		OutputDir:         request.OutputDir,
		Status:            TaskStatusQueued,
		QueuedAt:          time.Now().UTC().Format(time.RFC3339Nano),
	}
	q.tasks[taskID] = task
	q.requests[taskID] = request.DownloadRequest()
	q.order = append(q.order, taskID)
	q.pending = append(q.pending, taskID)
	q.mu.Unlock()

	q.emit(task)
	q.startAvailable()

	current, _ := q.Get(taskID)
	return current, nil
}

func (q *Queue) List() []DownloadTask {
	q.mu.Lock()
	defer q.mu.Unlock()

	tasks := make([]DownloadTask, 0, len(q.order))
	for _, taskID := range q.order {
		if task, ok := q.tasks[taskID]; ok {
			tasks = append(tasks, task)
		}
	}
	return tasks
}

func (q *Queue) Get(taskID string) (DownloadTask, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	task, ok := q.tasks[taskID]
	return task, ok
}

func (q *Queue) Cancel(taskID string) bool {
	q.mu.Lock()
	cancel, ok := q.active[taskID]
	q.mu.Unlock()
	if !ok {
		return false
	}
	cancel()
	return true
}

func (q *Queue) Stop() {
	q.mu.Lock()
	q.stopped = true
	cancels := make([]context.CancelFunc, 0, len(q.active))
	for _, cancel := range q.active {
		cancels = append(cancels, cancel)
	}
	q.mu.Unlock()

	for _, cancel := range cancels {
		cancel()
	}
}

func (q *Queue) startAvailable() {
	for {
		q.mu.Lock()
		if q.stopped || len(q.active) >= q.maxConcurrent || len(q.pending) == 0 {
			q.mu.Unlock()
			return
		}

		taskID := q.pending[0]
		q.pending = q.pending[1:]
		task := q.tasks[taskID]
		request := q.requests[taskID]
		task.Status = TaskStatusRunning
		task.StartedAt = time.Now().UTC().Format(time.RFC3339Nano)
		q.tasks[taskID] = task

		runCtx, cancel := context.WithCancel(q.parent)
		q.active[taskID] = cancel
		q.mu.Unlock()

		q.emit(task)
		go q.run(runCtx, taskID, request)
	}
}

func (q *Queue) run(ctx context.Context, taskID string, request DownloadRequest) {
	result, err := q.executor.Download(ctx, request, func(progress DownloadProgress) {
		q.updateProgress(taskID, progress)
	})

	q.mu.Lock()
	task, ok := q.tasks[taskID]
	if !ok {
		delete(q.active, taskID)
		q.mu.Unlock()
		q.startAvailable()
		return
	}

	task.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			task.Status = TaskStatusCancelled
		} else {
			task.Status = TaskStatusFailed
		}
		task.Error = err.Error()
	} else {
		task.Status = TaskStatusCompleted
		task.Progress = result.LastProgress
		task.FinalPath = result.FinalPath
	}
	q.tasks[taskID] = task
	delete(q.requests, taskID)
	delete(q.active, taskID)
	q.mu.Unlock()

	q.emit(task)
	q.startAvailable()
}

func (q *Queue) updateProgress(taskID string, progress DownloadProgress) {
	q.mu.Lock()
	task, ok := q.tasks[taskID]
	if !ok || task.Status != TaskStatusRunning {
		q.mu.Unlock()
		return
	}
	task.Progress = progress
	q.tasks[taskID] = task
	q.mu.Unlock()

	q.emit(task)
}

func (q *Queue) emit(task DownloadTask) {
	if q.onState != nil {
		q.onState(task)
	}
}
