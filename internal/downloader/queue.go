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

	mu              sync.Mutex
	tasks           map[string]DownloadTask
	requests        map[string]DownloadRequest
	order           []string
	pending         []string
	active          map[string]context.CancelFunc
	cancelRequested map[string]struct{}
	counter         atomic.Uint64
	stopped         bool
}

func NewQueue(parent context.Context, executor DownloadExecutor, onState TaskStateHandler, maxConcurrent int) *Queue {
	if parent == nil {
		parent = context.Background()
	}
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}
	return &Queue{
		parent:          parent,
		executor:        executor,
		onState:         onState,
		maxConcurrent:   maxConcurrent,
		tasks:           make(map[string]DownloadTask),
		requests:        make(map[string]DownloadRequest),
		active:          make(map[string]context.CancelFunc),
		cancelRequested: make(map[string]struct{}),
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

	taskID := fmt.Sprintf(
		"download-%d-%d-%d",
		request.VideoNo,
		time.Now().UTC().UnixNano(),
		q.counter.Add(1),
	)
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

func (q *Queue) MaxConcurrent() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.maxConcurrent
}

func (q *Queue) SetMaxConcurrent(maxConcurrent int) error {
	if maxConcurrent <= 0 {
		return fmt.Errorf("동시 다운로드 수는 1 이상이어야 합니다")
	}

	q.mu.Lock()
	if q.stopped {
		q.mu.Unlock()
		return fmt.Errorf("다운로드 Queue가 종료되었습니다")
	}
	q.maxConcurrent = maxConcurrent
	q.mu.Unlock()

	q.startAvailable()
	return nil
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
	task, ok := q.tasks[taskID]
	if !ok {
		q.mu.Unlock()
		return false
	}

	switch task.Status {
	case TaskStatusQueued:
		task.Status = TaskStatusCancelled
		task.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
		q.tasks[taskID] = task
		delete(q.requests, taskID)
		q.removePendingLocked(taskID)
		q.mu.Unlock()
		q.emit(task)
		q.startAvailable()
		return true

	case TaskStatusRunning:
		cancel, active := q.active[taskID]
		if !active {
			q.mu.Unlock()
			return false
		}
		q.cancelRequested[taskID] = struct{}{}
		task.Status = TaskStatusCancelled
		task.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
		task.Error = ""
		q.tasks[taskID] = task
		q.mu.Unlock()

		q.emit(task)
		cancel()
		return true

	default:
		q.mu.Unlock()
		return false
	}
}

func (q *Queue) Stop() {
	q.mu.Lock()
	if q.stopped {
		q.mu.Unlock()
		return
	}
	q.stopped = true

	now := time.Now().UTC().Format(time.RFC3339Nano)
	cancelledQueued := make([]DownloadTask, 0, len(q.pending))
	for _, taskID := range q.pending {
		task, ok := q.tasks[taskID]
		if !ok || task.Status != TaskStatusQueued {
			continue
		}
		task.Status = TaskStatusCancelled
		task.FinishedAt = now
		q.tasks[taskID] = task
		delete(q.requests, taskID)
		cancelledQueued = append(cancelledQueued, task)
	}
	q.pending = nil

	cancels := make([]context.CancelFunc, 0, len(q.active))
	for taskID, cancel := range q.active {
		q.cancelRequested[taskID] = struct{}{}
		cancels = append(cancels, cancel)
	}
	q.mu.Unlock()

	for _, task := range cancelledQueued {
		q.emit(task)
	}
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
		task, ok := q.tasks[taskID]
		if !ok || task.Status != TaskStatusQueued {
			q.mu.Unlock()
			continue
		}
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
		delete(q.cancelRequested, taskID)
		q.mu.Unlock()
		q.startAvailable()
		return
	}

	_, cancellationRequested := q.cancelRequested[taskID]
	alreadyCancelled := task.Status == TaskStatusCancelled
	if !alreadyCancelled {
		task.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if cancellationRequested || alreadyCancelled {
		task.Status = TaskStatusCancelled
		if err != nil {
			task.Error = err.Error()
		}
	} else if err != nil {
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
	delete(q.cancelRequested, taskID)
	q.mu.Unlock()

	if !alreadyCancelled {
		q.emit(task)
	}
	q.startAvailable()
}

func (q *Queue) Remove(taskID string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	task, ok := q.tasks[taskID]
	if !ok {
		return false
	}
	switch task.Status {
	case TaskStatusCompleted, TaskStatusFailed, TaskStatusCancelled:
	default:
		return false
	}

	delete(q.tasks, taskID)
	delete(q.requests, taskID)
	delete(q.cancelRequested, taskID)
	for index, currentTaskID := range q.order {
		if currentTaskID == taskID {
			q.order = append(q.order[:index], q.order[index+1:]...)
			break
		}
	}
	q.removePendingLocked(taskID)
	return true
}

func (q *Queue) updateProgress(taskID string, progress DownloadProgress) {
	q.mu.Lock()
	task, ok := q.tasks[taskID]
	_, cancellationRequested := q.cancelRequested[taskID]
	if !ok || task.Status != TaskStatusRunning || cancellationRequested {
		q.mu.Unlock()
		return
	}
	task.Progress = progress
	q.tasks[taskID] = task
	q.mu.Unlock()

	q.emit(task)
}

func (q *Queue) removePendingLocked(taskID string) {
	for index, pendingTaskID := range q.pending {
		if pendingTaskID != taskID {
			continue
		}
		q.pending = append(q.pending[:index], q.pending[index+1:]...)
		return
	}
}

func (q *Queue) emit(task DownloadTask) {
	if q.onState != nil {
		q.onState(task)
	}
}
