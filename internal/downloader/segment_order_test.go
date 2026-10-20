package downloader

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// orderedClipExecutor keeps jobs running until their individual gates open.
type orderedClipExecutor struct {
	mu      sync.Mutex
	started []string
	gates   map[string]chan struct{}
}

func newOrderedClipExecutor() *orderedClipExecutor {
	return &orderedClipExecutor{gates: make(map[string]chan struct{})}
}

func orderedClipKey(request DownloadRequest) string {
	if request.Segment != nil {
		return fmt.Sprintf("%s/clip-%d", request.URL, request.Segment.Position)
	}
	return request.URL
}

func (e *orderedClipExecutor) Download(ctx context.Context, request DownloadRequest, _ ProgressHandler) (DownloadResult, error) {
	key := orderedClipKey(request)
	gate := make(chan struct{})
	e.mu.Lock()
	e.gates[key] = gate
	e.started = append(e.started, key)
	e.mu.Unlock()
	select {
	case <-ctx.Done():
		return DownloadResult{}, ctx.Err()
	case <-gate:
		return DownloadResult{
			FinalPath:    key + ".mp4",
			LastProgress: DownloadProgress{Status: "completed", Percent: 100},
		}, nil
	}
}

func (e *orderedClipExecutor) release(key string) {
	e.mu.Lock()
	gate := e.gates[key]
	e.mu.Unlock()
	if gate != nil {
		close(gate)
	}
}

func (e *orderedClipExecutor) snapshot() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.started...)
}

func awaitStartedClipCount(t *testing.T, e *orderedClipExecutor, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if len(e.snapshot()) >= want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("expected %d started jobs, got %v", want, e.snapshot())
}

func activateOrderedClip(t *testing.T, queue *Queue, videoNo int64, position int) DownloadTask {
	t.Helper()
	request := queueRequest(videoNo)
	request.Segment = &DownloadSegment{
		Title: fmt.Sprintf("clip-%d", position), Position: position,
		StartSeconds: int64(position * 60),
	}
	prepared, err := queue.Enqueue(request)
	if err != nil {
		t.Fatal(err)
	}
	activated, err := queue.ActivatePrepared(prepared.TaskID, queue.requests[prepared.TaskID])
	if err != nil {
		t.Fatal(err)
	}
	return activated
}

func TestSegmentDownloadsRunSequentiallyForSameVODWithMultipleWorkers(t *testing.T) {
	executor := newOrderedClipExecutor()
	queue := NewQueue(context.Background(), executor, nil, 3)
	defer queue.Stop()

	first := activateOrderedClip(t, queue, 90801, 1)
	second := activateOrderedClip(t, queue, 90801, 2)
	third := activateOrderedClip(t, queue, 90801, 3)

	// A second VOD is free to run beside the first clip.
	unrelated, err := queue.Enqueue(queueRequest(90802))
	if err != nil {
		t.Fatal(err)
	}
	awaitStartedClipCount(t, executor, 2)
	if first.Status != TaskStatusRunning {
		t.Fatalf("first clip must start immediately: %s", first.Status)
	}
	if task, _ := queue.Get(second.TaskID); task.Status != TaskStatusQueued {
		t.Fatalf("second clip must wait: %s", task.Status)
	}
	if task, _ := queue.Get(third.TaskID); task.Status != TaskStatusQueued {
		t.Fatalf("third clip must wait: %s", task.Status)
	}

	firstKey := "https://chzzk.naver.com/video/90801/clip-1"
	secondKey := "https://chzzk.naver.com/video/90801/clip-2"
	thirdKey := "https://chzzk.naver.com/video/90801/clip-3"
	executor.release(firstKey)
	waitForTaskStatus(t, queue, first.TaskID, TaskStatusCompleted)
	waitForTaskStatus(t, queue, second.TaskID, TaskStatusRunning)
	awaitStartedClipCount(t, executor, 3)
	if task, _ := queue.Get(third.TaskID); task.Status != TaskStatusQueued {
		t.Fatal("third clip started before second completed")
	}

	executor.release(secondKey)
	waitForTaskStatus(t, queue, second.TaskID, TaskStatusCompleted)
	waitForTaskStatus(t, queue, third.TaskID, TaskStatusRunning)
	awaitStartedClipCount(t, executor, 4)
	executor.release(thirdKey)
	executor.release(unrelated.URL)
	waitForTaskStatus(t, queue, third.TaskID, TaskStatusCompleted)
	waitForTaskStatus(t, queue, unrelated.TaskID, TaskStatusCompleted)

	got := executor.snapshot()
	want := []string{firstKey, unrelated.URL, secondKey, thirdKey}
	if len(got) != len(want) {
		t.Fatalf("started %v, wanted %v", got, want)
	}
	// Another VOD may start between clips, but the clip subsequence is FIFO.
	clipStarts := []string{}
	for _, name := range got {
		if name != unrelated.URL {
			clipStarts = append(clipStarts, name)
		}
	}
	for i, name := range []string{firstKey, secondKey, thirdKey} {
		if clipStarts[i] != name {
			t.Fatalf("clip order incorrect: %v", clipStarts)
		}
	}
}

func TestQueuedSegmentCancellationSkipsToNextClip(t *testing.T) {
	executor := newOrderedClipExecutor()
	queue := NewQueue(context.Background(), executor, nil, 3)
	defer queue.Stop()

	first := activateOrderedClip(t, queue, 90811, 1)
	second := activateOrderedClip(t, queue, 90811, 2)
	third := activateOrderedClip(t, queue, 90811, 3)
	if !queue.Cancel(second.TaskID) {
		t.Fatal("queued clip cancellation failed")
	}
	awaitStartedClipCount(t, executor, 1)
	executor.release("https://chzzk.naver.com/video/90811/clip-1")
	waitForTaskStatus(t, queue, first.TaskID, TaskStatusCompleted)
	waitForTaskStatus(t, queue, third.TaskID, TaskStatusRunning)
	awaitStartedClipCount(t, executor, 2)
	executor.release("https://chzzk.naver.com/video/90811/clip-3")
	waitForTaskStatus(t, queue, third.TaskID, TaskStatusCompleted)
	if len(executor.snapshot()) != 2 {
		t.Fatalf("cancelled clip ran unexpectedly: %v", executor.snapshot())
	}
}
