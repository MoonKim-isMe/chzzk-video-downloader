package downloader

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestMP3MetadataTagsForFullAndSegmentDownloads(t *testing.T) {
	for _, tc := range []struct {
		name    string
		segment *DownloadSegment
		title   string
		track   string
	}{
		{name: "full", title: "title=엘든링의 고수가 되고 싶어요"},
		{name: "segment", segment: &DownloadSegment{Title: "보스전 하이라이트", Position: 2}, title: "title=보스전 하이라이트", track: "track=2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := DownloadRequest{
				DownloadType: DownloadTypeAudio, VideoTitle: "엘든링의 고수가 되고 싶어요",
				PublishDate: "2026-11-19 21:30:45", ChannelName: "테스트 채널",
				Segment: tc.segment,
			}
			tags := mp3MetadataTags(request)
			for _, expected := range []string{tc.title, "album=엘든링의 고수가 되고 싶어요", "artist=테스트 채널", "date=2026-11-19"} {
				if !slices.Contains(tags, expected) {
					t.Fatalf("missing %q from %#v", expected, tags)
				}
			}
			if tc.track != "" && !slices.Contains(tags, tc.track) {
				t.Fatalf("missing track %q from %#v", tc.track, tags)
			}
			if tc.track == "" && slices.Contains(tags, "track=2") {
				t.Fatalf("unexpected track in whole VOD: %#v", tags)
			}
			spec, ok := buildMP3MetadataCommand("ffmpeg", "source.mp3", "target.mp3", request)
			if !ok || !hasArgumentPair(spec.Args, "-c:a", "copy") ||
				!hasArgumentPair(spec.Args, "-id3v2_version", "3") {
				t.Fatalf("metadata step must copy MP3 without re-encoding: %#v", spec)
			}
			if slices.Contains(spec.Args, "libmp3lame") {
				t.Fatal("MP3 metadata update must not re-encode audio")
			}
		})
	}
}

func TestMP3PublicationDateDoesNotInventBroadcastTime(t *testing.T) {
	for _, tc := range []struct { raw, want string }{
		{"2026-11-19T17:30:00+09:00", "2026-11-19"},
		{"2026. 11. 19. 17:30", "2026-11-19"},
		{"2026/1/2", "2026-01-02"},
		{"2026-02-30", ""},
		{"", ""},
		{"not a date", ""},
	} {
		if got := mp3PublicationDate(tc.raw); got != tc.want {
			t.Errorf("mp3PublicationDate(%q) = %q; want %q", tc.raw, got, tc.want)
		}
	}
	if slices.Contains(mp3MetadataTags(DownloadRequest{VideoTitle: "제목", PublishDate: "invalid"}), "date=invalid") {
		t.Fatal("unknown date must not be written")
	}
}

func TestApplyMP3MetadataPreservesOriginalWhenFFmpegFails(t *testing.T) {
	dir := t.TempDir()
	original := filepath.Join(dir, "original.mp3")
	if err := os.WriteFile(original, []byte("original audio"), 0600); err != nil {
		t.Fatal(err)
	}
	err := applyMP3Metadata(context.Background(), "ffmpeg", DownloadRequest{
		DownloadType: DownloadTypeAudio, VideoTitle: "타이틀",
	}, original, func(_ context.Context, _ CommandSpec, _ LineHandler) error { return errors.New("ffmpeg failed") })
	if err == nil {
		t.Fatal("expected FFmpeg error")
	}
	contents, err := os.ReadFile(original)
	if err != nil || string(contents) != "original audio" {
		t.Fatalf("failed tag update must preserve original file, data=%q err=%v", contents, err)
	}
	leftovers, err := filepath.Glob(filepath.Join(dir, ".chzzk-mp3-metadata-*.mp3"))
	if err != nil || len(leftovers) != 0 {
		t.Fatalf("temporary metadata files were not removed: %v %v", leftovers, err)
	}
}

func TestApplyMP3MetadataWithRealFFmpegWhenAvailable(t *testing.T) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil { t.Skip("FFmpeg not on PATH") }
	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil { t.Skip("FFprobe not on PATH") }

	original := filepath.Join(t.TempDir(), "샘플.mp3")
	generate := exec.Command(ffmpegPath, "-hide_banner", "-loglevel", "error", "-f", "lavfi",
		"-i", "sine=frequency=440:duration=0.3", "-c:a", "libmp3lame", original)
	if err := generate.Run(); err != nil { t.Skipf("FFmpeg test encoder unavailable: %v", err) }
	request := DownloadRequest{DownloadType: DownloadTypeAudio,
		VideoTitle: "한국어 방송 제목", ChannelName: "방송인", PublishDate: "2026-11-19T21:30:00",
		Segment: &DownloadSegment{Title: "구간 제목", Position: 3}}
	if err := applyMP3Metadata(context.Background(), ffmpegPath, request, original, NewRunner().Run); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(ffprobePath, "-v", "error", "-show_entries", "format_tags",
		"-of", "json", original).Output()
	if err != nil { t.Fatal(err) }
	var info struct { Format struct { Tags map[string]string `json:"tags"` } `json:"format"` }
	if err := json.Unmarshal(output, &info); err != nil { t.Fatal(err) }
	for key, want := range map[string]string{
		"title": "구간 제목", "album": "한국어 방송 제목", "artist": "방송인",
		"date": "2026-11-19", "track": "3",
	} {
		if got := info.Format.Tags[key]; !strings.EqualFold(got, want) {
			t.Errorf("%s = %q; expected %q; tags=%#v", key, got, want, info.Format.Tags)
		}
	}
}
