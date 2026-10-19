package downloader

import (
  "context"
  "os"
  "path/filepath"
  "strings"
  "testing"

  appsettings "github.com/MoonKim-isMe/chzzk-video-downloader/internal/settings"
)

func TestSegmentNameSanitizationAndOutputFolder(t *testing.T) {
  root := t.TempDir()
  request, err := ApplySettings(StartDownloadRequest{
    VideoNo: 123, VideoTitle: "VOD: test / 2026?",
    URL: "https://chzzk.naver.com/video/123",
    Segment: &DownloadSegment{Title: "part", Position: 1, StartSeconds: 0},
  }, appsettings.AppSettings{
    DownloadDir: root, Resolution: appsettings.ResolutionBest,
    OutputFormat: appsettings.OutputFormatMP4,
    MaxConcurrentDownloads: 3,
  })
  if err != nil { t.Fatal(err) }
  expected := filepath.Join(root, "VOD- test - 2026-")
  if request.OutputDir != expected {
    t.Fatalf("wrong VOD directory: %q want %q", request.OutputDir, expected)
  }
  if sanitizeSegmentName("../foo") != "-foo" {
    t.Fatalf("unsafe filename sanitization: %q", sanitizeSegmentName("../foo"))
  }
  if sanitizeSegmentName("CON") != "_CON" {
    t.Fatal("Windows reserved filename not escaped")
  }
  if sanitizeSegmentName(" . ") != "제목 없음" {
    t.Fatal("blank filename did not get fallback")
  }
  full, err := ApplySettings(StartDownloadRequest{
    VideoNo: 123, VideoTitle: "VOD: test / 2026?",
    URL: "https://chzzk.naver.com/video/123",
  }, appsettings.Defaults(root))
  if err != nil { t.Fatal(err) }
  if full.OutputDir != root { t.Fatal("non-segment output directory must be unchanged") }
}

func TestSegmentQueuePreservesIndependentPreparedRecordsAndFilenames(t *testing.T) {
  root := t.TempDir()
  executor := newControlledExecutor()
  queue := NewQueue(context.Background(), executor, nil, 2)
  base := queueRequest(81234)
  base.OutputDir = root
  end := int64(180)
  base.Segment = &DownloadSegment{Title: "동일 제목", Position: 1, StartSeconds: 10, EndSeconds: &end}
  first, err := queue.Enqueue(base)
  if err != nil { t.Fatal(err) }
  second, err := queue.Enqueue(base)
  if err != nil { t.Fatal(err) }
  if first.TaskID == second.TaskID || first.Segment.FileName == second.Segment.FileName {
    t.Fatalf("prepared segment jobs not isolated: %#v %#v", first, second)
  }
  if first.Status != TaskStatusPrepared || second.Status != TaskStatusPrepared || executor.startedCount() != 0 {
    t.Fatal("prepared segments must not start a full-VOD download")
  }
  if first.Segment.FileName != "01. 동일 제목" || second.Segment.FileName != "01. 동일 제목 (2)" {
    t.Fatalf("unexpected reserved names: %q %q", first.Segment.FileName, second.Segment.FileName)
  }
  if queue.requests[first.TaskID].OutputTemplate != "01. 동일 제목.%(ext)s" {
    t.Fatal("segment output template not isolated")
  }
  full := queueRequest(81234)
  full.OutputDir = root
  started, err := queue.Enqueue(full)
  if err != nil { t.Fatal(err) }
  if started.Status != TaskStatusRunning {
    t.Fatal("prepared segments must not block whole-VOD jobs")
  }
  waitForStartedCount(t, executor, 1)
  if _, err := queue.Enqueue(full); err == nil {
    t.Fatal("duplicate full-VOD download should remain rejected")
  }
  queue.Stop()
  saved, found := queue.Get(first.TaskID)
  if !found || saved.Status != TaskStatusPrepared {
    t.Fatal("prepared segment should be preserved on shutdown")
  }
  if !queue.Remove(first.TaskID) { t.Fatal("prepared segment cannot be removed") }

  otherQueue := NewQueue(context.Background(), executor, nil, 1)
  if err := os.WriteFile(filepath.Join(root, "01. 동일 제목.mp4"), []byte("existing"), 0o644); err != nil {
    t.Fatal(err)
  }
  another, err := otherQueue.Enqueue(base)
  if err != nil { t.Fatal(err) }
  if !strings.HasSuffix(another.Segment.FileName, "(2)") {
    t.Fatalf("existing output file not respected: %q", another.Segment.FileName)
  }
}

func TestSegmentRequestValidation(t *testing.T) {
  valid := queueRequest(230)
  valid.Segment = &DownloadSegment{Title: "Overlap", Position: 2, StartSeconds: 10}
  if err := valid.Validate(); err != nil { t.Fatal(err) }
  for _, bad := range []*DownloadSegment{
    {Title: "", Position: 1, StartSeconds: 10},
    {Title: "Bad position", Position: 0, StartSeconds: 10},
    {Title: "Bad start", Position: 1, StartSeconds: -1},
  } {
    valid.Segment = bad
    if err := valid.Validate(); err == nil {
      t.Fatalf("invalid segment accepted: %#v", bad)
    }
  }
  end := int64(9)
  valid.Segment = &DownloadSegment{Title: "Bad end", Position: 1, StartSeconds: 10, EndSeconds: &end}
  if err := valid.Validate(); err == nil { t.Fatal("end before start was accepted") }
}


func TestSegmentNumberFileNameOptionAndCollision(t *testing.T) {
  root := t.TempDir()
  q := NewQueue(context.Background(), newControlledExecutor(), nil, 2)
  noNumber := false
  numbered := true

  cases := []struct {
    segment DownloadSegment
    expected string
  }{
    {DownloadSegment{Title: "영상1", Position: 1, NumberFileName: &numbered, StartSeconds: 0}, "01. 영상1"},
    {DownloadSegment{Title: "영상2", Position: 2, NumberFileName: &numbered, StartSeconds: 90}, "02. 영상2"},
    {DownloadSegment{Title: "영상1", Position: 1, NumberFileName: &noNumber, StartSeconds: 0}, "영상1"},
    {DownloadSegment{Title: "영상2", Position: 2, NumberFileName: &noNumber, StartSeconds: 90}, "영상2"},
    {DownloadSegment{Title: "영상1", Position: 3, NumberFileName: &noNumber, StartSeconds: 0}, "영상1 (2)"},
    {DownloadSegment{Title: "기존 방식", Position: 6, StartSeconds: 40}, "06. 기존 방식"},
    {DownloadSegment{Title: "자릿수 확장", Position: 100, StartSeconds: 70}, "100. 자릿수 확장"},
  }
  for _, tt := range cases {
    req := queueRequest(81234)
    req.OutputDir = root
    req.Segment = &tt.segment
    got, err := q.Enqueue(req)
    if err != nil { t.Fatal(err) }
    if got.Segment.FileName != tt.expected {
      t.Fatalf("position %d: got %q, want %q", tt.segment.Position, got.Segment.FileName, tt.expected)
    }
    if got.Segment.NumberFileName == nil && tt.segment.NumberFileName != nil {
      t.Fatalf("numbering option not retained: %#v", got.Segment)
    }
  }
  if err := os.WriteFile(filepath.Join(root, "영상2.mp4"), []byte("existing"), 0o644); err != nil {
    t.Fatal(err)
  }
  q2 := NewQueue(context.Background(), newControlledExecutor(), nil, 2)
  req := queueRequest(81234)
  req.OutputDir = root
  req.Segment = &DownloadSegment{Title: "영상2", Position: 1, NumberFileName: &noNumber, StartSeconds: 100}
  got, err := q2.Enqueue(req)
  if err != nil { t.Fatal(err) }
  if got.Segment.FileName != "영상2 (2)" {
    t.Fatalf("existing file collision ignored: %s", got.Segment.FileName)
  }
}
