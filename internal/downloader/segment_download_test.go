package downloader

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestYTDownloadSectionVideoAndMP3(t *testing.T) {
	end := int64(413) // 6:53 inclusive -> 6:54 exclusive.
	base := DownloadRequest{
		URL: "https://chzzk.naver.com/video/12345",
		OutputDir: t.TempDir(),
		OutputTemplate: "01 - 첫 영상.%(ext)s",
		Segment: &DownloadSegment{Title: "첫 영상", Position: 1, FileName: "01 - 첫 영상", StartSeconds: 179, EndSeconds: &end},
	}
	for _, kind := range []DownloadType{DownloadTypeVideo, DownloadTypeAudio} {
		req := base
		req.DownloadType = kind
		cmd, err := BuildDownloadCommand(readyToolchain(t.TempDir()), req)
		if err != nil { t.Fatal(err) }
		if !hasArgumentPair(cmd.Args, "--download-sections", "*00:02:59-00:06:54") {
			t.Fatalf("%s: missing selected range: %v", kind, cmd.Args)
		}
		if !hasArgumentPair(cmd.Args, "--output", "01 - 첫 영상.%(ext)s") {
			t.Fatalf("%s: wrong output file template", kind)
		}
		if kind == DownloadTypeAudio {
			if !slices.Contains(cmd.Args, "--extract-audio") || !hasArgumentPair(cmd.Args, "--audio-format", "mp3") {
				t.Fatal("MP3 segment lost audio conversion")
			}
		} else if !slices.Contains(cmd.Args, "--force-keyframes-at-cuts") {
			t.Fatal("video segment should cut accurately")
		}
	}
	openEnded := &DownloadSegment{Title: "last", Position: 2, StartSeconds: 472}
	start, stop := ytDLPSectionRange(openEnded)
	if start != "00:07:52" || stop != "inf" {
		t.Fatalf("unexpected open-ended section: %s-%s", start, stop)
	}
}

func TestSourceURLDirectSegmentFFmpegVideoAndMP3(t *testing.T) {
	end := int64(472)
	status := readyToolchain(t.TempDir())
	playlist := filepath.Join(t.TempDir(), "playlist.m3u8")
	stream := sourceURLHLSStream{PlaylistPath: playlist, Duration: 480, VideoNo: 12345}
	for _, kind := range []DownloadType{DownloadTypeVideo, DownloadTypeAudio} {
		ext := "mp4"
		if kind == DownloadTypeAudio { ext = "mp3" }
		req := DownloadRequest{
			DownloadType: kind, OutputFormat: "mp4",
			Segment: &DownloadSegment{Title: "video", Position: 1, StartSeconds: 179, EndSeconds: &end},
		}
		out := filepath.Join(t.TempDir(), "01 - video."+ext)
		spec, err := buildSourceURLFFmpegCommand(status, req, stream, out)
		if err != nil { t.Fatal(err) }
		if !hasArgumentPair(spec.Args, "-ss", "00:02:59") || !hasArgumentPair(spec.Args, "-t", "294") {
			t.Fatalf("%s: wrong ffmpeg range: %v", kind, spec.Args)
		}
		if !hasArgumentPair(spec.Args, "-i", playlist) {
			t.Fatal("sourceURL must retain the authorized local manifest")
		}
		if kind == DownloadTypeVideo {
			if !hasArgumentPair(spec.Args, "-c", "copy") || slices.Contains(spec.Args, "libx264") {
				t.Fatal("video clip must initially attempt lossless stream copy")
			}
		} else if !hasArgumentPair(spec.Args, "-codec:a", "libmp3lame") {
			t.Fatal("MP3 clip must encode audio only")
		}
	}
}

func TestSourceURLPrefetchOnlyIntersectsSelectedTimes(t *testing.T) {
	playlist := strings.Join([]string{
		"#EXTM3U", "#EXT-X-TARGETDURATION:4",
		"#EXTINF:4.0,", "http://127.0.0.1:3456/hls/1.ts",
		"#EXTINF:4.0,", "http://127.0.0.1:3456/hls/2.ts",
		"#EXTINF:4.0,", "http://127.0.0.1:3456/hls/3.ts",
		"#EXT-X-ENDLIST",
	}, "\n")
	selected, ok := segmentPrefetchURLs(playlist, 4, 8, true)
	if !ok || len(selected) != 1 || !strings.HasSuffix(selected[0], "/2.ts") {
		t.Fatalf("wrong prefetch region: %v %v", selected, ok)
	}
	open, ok := segmentPrefetchURLs(playlist, 8, 0, false)
	if !ok || len(open) != 1 || !strings.HasSuffix(open[0], "/3.ts") {
		t.Fatalf("wrong open-ended prefetch: %v %v", open, ok)
	}
	if _, ok = segmentPrefetchURLs("#EXTM3U\n#EXTINF:bad,\nuri.ts", 4, 8, true); ok {
		t.Fatal("invalid segment durations should disable speculative filtering")
	}
}

func TestQueueActivatePreparedSegmentsRespectsConcurrency(t *testing.T) {
	queue := NewQueue(context.Background(), &parsedProgressExecutor{}, nil, 2)
	for position := 1; position <= 5; position++ {
		request := queueRequest(55777)
		request.Segment = &DownloadSegment{
			Title: "episode", Position: position, StartSeconds: int64(position * 60),
		}
		prepared, err := queue.Enqueue(request)
		if err != nil { t.Fatal(err) }
		if prepared.Status != TaskStatusPrepared { t.Fatal("not prepared") }
		activated, err := queue.ActivatePrepared(prepared.TaskID, queue.requests[prepared.TaskID])
		if err != nil { t.Fatal(err) }
		if activated.Status != TaskStatusQueued && activated.Status != TaskStatusRunning && activated.Status != TaskStatusCompleted {
			t.Fatalf("not runnable: %#v", activated)
		}
		waitForTaskStatus(t, queue, prepared.TaskID, TaskStatusCompleted)
		if _, err := queue.ActivatePrepared(prepared.TaskID, queue.requests[prepared.TaskID]); err == nil {
			t.Fatal("completed clip unexpectedly reactivated")
		}
	}
}

func TestSegmentTemporaryRootsAreIsolated(t *testing.T) {
	root := t.TempDir()
	makeRequest := func(name string) DownloadRequest {
		return DownloadRequest{
			URL: "https://chzzk.naver.com/video/12345", OutputDir: root,
			Segment: &DownloadSegment{Title: "clip", Position: 1, StartSeconds: 90, FileName: name},
		}
	}
	one := makeRequest("01 - A")
	two := makeRequest("01 - A (2)")
	a, err := temporaryRootForRequest(one)
	if err != nil { t.Fatal(err) }
	b, err := temporaryRootForRequest(two)
	if err != nil { t.Fatal(err) }
	if a == b || !strings.HasPrefix(a, root) || !strings.HasPrefix(b, root) {
		t.Fatalf("segment temporary roots collide: %s and %s", a, b)
	}
	if segmentDuration(one.Segment, 120) != 30 {
		t.Fatal("end-of-video range should use remaining duration")
	}
}
