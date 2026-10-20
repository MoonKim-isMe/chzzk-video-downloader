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
		if request.Segment == nil && task.Segment == nil && task.VideoNo == request.VideoNo &&
			NormalizeDownloadType(task.DownloadType) == NormalizeDownloadType(request.DownloadType) &&
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
	if request.Segment != nil {
		// A reserved, collision-free basename belongs to one independent job.
		segment := *request.Segment
		segment.FileName = q.reserveSegmentFileName(request.OutputDir, segment)
		request.Segment = &segment
		request.OutputTemplate = segment.FileName + ".%(ext)s"
	}
	status := TaskStatusQueued
	if request.Segment != nil {
		// Stage 2 registers a persistent prepared item. Stage 3 will make it
		// runnable once real time-range extraction supports every fallback.
		status = TaskStatusPrepared
	}
	task := DownloadTask{
		Segment:           request.Segment,
		DownloadType:      NormalizeDownloadType(request.DownloadType),
		TaskID:            taskID,
		VideoNo:           request.VideoNo,
		VideoTitle:        request.VideoTitle,
		ChannelName:       request.ChannelName,
		ThumbnailImageURL: request.ThumbnailImageURL,
		URL:               request.URL,
		OutputDir:         request.OutputDir,
		Status:            status,
		QueuedAt:          time.Now().UTC().Format(time.RFC3339Nano),
	}
	q.tasks[taskID] = task
	q.requests[taskID] = request.DownloadRequest()
	q.order = append(q.order, taskID)
	if request.Segment == nil {
		q.pending = append(q.pending, taskID)
	}
	q.mu.Unlock()

	q.emit(task)
	q.startAvailable()

	current, _ := q.Get(taskID)
	return current, nil
}

// RestorePreparedTasks reserves the existing filenames of segment drafts
// when restarting the application, without scheduling incomplete downloads.
func (q *Queue) RestorePreparedTasks(tasks []DownloadTask) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, task := range tasks {
		if task.Status != TaskStatusPrepared || task.Segment == nil || task.TaskID == "" {
			continue
		}
		if _, exists := q.tasks[task.TaskID]; exists {
			continue
		}
		q.tasks[task.TaskID] = task
		q.order = append(q.order, task.TaskID)
	}
}

// ActivatePrepared keeps the previously reserved name and output directory.
// The caller must have validated the request and access permissions.
func (q *Queue) ActivatePrepared(taskID string, request DownloadRequest) (DownloadTask, error) {
	q.mu.Lock()
	if q.stopped {
		q.mu.Unlock()
		return DownloadTask{}, fmt.Errorf("다운로드 Queue가 종료되었습니다")
	}
	task, exists := q.tasks[taskID]
	if !exists || task.Status != TaskStatusPrepared || task.Segment == nil {
		q.mu.Unlock()
		return DownloadTask{}, fmt.Errorf("시작할 준비된 구간 작업을 찾을 수 없습니다")
	}
	// Never trust caller-controlled output filename, folder or segment bounds
	// when starting a persisted draft.
	request.OutputDir = task.OutputDir
	request.URL = task.URL
	request.DownloadType = task.DownloadType
	request.Segment = task.Segment
	request.OutputTemplate = task.Segment.FileName + ".%(ext)s"
	task.Status = TaskStatusQueued
	q.tasks[taskID] = task
	q.requests[taskID] = request
	q.pending = append(q.pending, taskID)
	q.mu.Unlock()
	q.emit(task)
	q.startAvailable()
	started, _ := q.Get(taskID)
	return started, nil
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
		task.ErrorCode = ""
		task.LogPath = ""
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
		task.Status = TaskStatusFailed
		task.FinishedAt = now
		task.Error = "앱 종료로 다운로드가 중단되었습니다."
		task.ErrorCode = DownloadFailureInterrupted
		q.tasks[taskID] = task
		delete(q.requests, taskID)
		cancelledQueued = append(cancelledQueued, task)
	}
	q.pending = nil

	cancels := make([]context.CancelFunc, 0, len(q.active))
	for _, cancel := range q.active {
		// An app shutdown is not an explicit user cancellation.
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

		// Preserve FIFO within each VOD's segment jobs. A clip must wait for
		// the preceding clip to finish even when the global concurrency limit
		// permits more workers. Jobs from unrelated VODs can use free slots.
		nextIndex := -1
		for i := 0; i < len(q.pending); {
			candidateID := q.pending[i]
			candidate, ok := q.tasks[candidateID]
			if !ok || candidate.Status != TaskStatusQueued {
				q.pending = append(q.pending[:i], q.pending[i+1:]...)
				continue
			}
			blocked := false
			if candidate.Segment != nil {
				for activeID := range q.active {
					activeTask, exists := q.tasks[activeID]
					if exists && activeTask.Segment != nil && activeTask.VideoNo == candidate.VideoNo {
						blocked = true
						break
					}
				}
			}
			if !blocked {
				nextIndex = i
				break
			}
			i++
		}
		if nextIndex < 0 {
			q.mu.Unlock()
			return
		}
		taskID := q.pending[nextIndex]
		q.pending = append(q.pending[:nextIndex], q.pending[nextIndex+1:]...)
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
		task.Status = TaskStatusFailed
		task.Error = err.Error()
		task.ErrorCode = ""
		task.LogPath = result.LogPath
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			// Only Cancel() represents an intentional user cancellation.
			message := "다운로드 실행이 예기치 않게 중단되었습니다."
			if q.stopped {
				message = "앱 종료로 다운로드가 중단되었습니다."
			}
			interruption := &DownloadFailure{
				Kind: DownloadFailureInterrupted, Message: message, Cause: err,
			}
			task.Error = message
			task.ErrorCode = interruption.Kind
			// Manager intentionally skips logs for user cancellation. Keep
			// a failure log when a non-user interruption is detected.
			if logPath, logErr := writeDownloadFailureLog(request, CommandSpec{}, result, interruption); logErr == nil {
				task.LogPath = logPath
			}
		} else if kind, ok := downloadFailureKind(err); ok {
			task.ErrorCode = kind
		}
	} else {
		task.Status = TaskStatusCompleted
		task.Error = ""
		task.ErrorCode = ""
		task.LogPath = ""
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
	case TaskStatusPrepared, TaskStatusCompleted, TaskStatusFailed, TaskStatusCancelled:
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
