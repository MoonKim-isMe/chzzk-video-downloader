package downloader

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestBuildHLSFormatProbeCommandUsesOriginalSelector(t *testing.T) {
	toolchain := readyToolchain(t.TempDir())
	spec, err := buildHLSFormatProbeCommand(toolchain, DownloadRequest{
		URL:            "https://chzzk.naver.com/video/15461111",
		OutputDir:      t.TempDir(),
		FormatSelector: "bv*[height<=1080]+ba/b[height<=1080]",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !hasArgumentPair(spec.Args, "--format", "bv*[height<=1080]+ba/b[height<=1080]") {
		t.Fatalf("format selector missing: %#v", spec.Args)
	}
	if !slices.Contains(spec.Args, "--simulate") {
		t.Fatalf("format probe must not download media: %#v", spec.Args)
	}
}

func TestProbeHLSSelectedFormatsPrefersRequestedFormats(t *testing.T) {
	formats, _, err := probeHLSSelectedFormats(
		context.Background(),
		CommandSpec{Path: "yt-dlp.exe"},
		func(_ context.Context, _ CommandSpec, handler LineHandler) error {
			handler(OutputLine{
				Stream: StreamStdout,
				Text: hlsFormatProbeRequestedPrefix +
					`[{"format_id":"video","vcodec":"avc1","acodec":"none","ext":"mp4"},{"format_id":"audio","vcodec":"none","acodec":"mp4a","ext":"m4a"}]`,
			})
			handler(OutputLine{
				Stream: StreamStdout,
				Text: hlsFormatProbeSinglePrefix +
					`{"format_id":"video+audio","vcodec":"avc1","acodec":"mp4a","ext":"mp4"}`,
			})
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(formats) != 2 || formats[0].FormatID != "video" || formats[1].FormatID != "audio" {
		t.Fatalf("unexpected requested formats: %#v", formats)
	}
}

func TestProbeHLSSelectedFormatsFallsBackToSingleFormat(t *testing.T) {
	formats, _, err := probeHLSSelectedFormats(
		context.Background(),
		CommandSpec{Path: "yt-dlp.exe"},
		func(_ context.Context, _ CommandSpec, handler LineHandler) error {
			handler(OutputLine{
				Stream: StreamStdout,
				Text: hlsFormatProbeSinglePrefix +
					`{"format_id":"1080p","vcodec":"avc1","acodec":"mp4a","ext":"mp4"}`,
			})
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(formats) != 1 || formats[0].FormatID != "1080p" {
		t.Fatalf("unexpected single format: %#v", formats)
	}
}

func TestProbeHLSSelectedFormatsReturnsProbeError(t *testing.T) {
	_, _, err := probeHLSSelectedFormats(
		context.Background(),
		CommandSpec{Path: "yt-dlp.exe"},
		func(context.Context, CommandSpec, LineHandler) error {
			return errors.New("probe failed")
		},
	)
	if err == nil || !strings.Contains(err.Error(), "구간 분할용 포맷") {
		t.Fatalf("unexpected probe error: %v", err)
	}
}

func TestBuildDiscontinuityFormatSelectorSelectsAllSiblingSegments(t *testing.T) {
	selector, err := buildDiscontinuityFormatSelector([]hlsFallbackFormat{
		{FormatID: "1080p"},
		{FormatID: "audio-main"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"all[format_id~='^1080p(?:-[0-9]+)?$']",
		"all[format_id~='^audio-main(?:-[0-9]+)?$']",
	} {
		if !strings.Contains(selector, expected) {
			t.Fatalf("missing selector %q in %q", expected, selector)
		}
	}
}

func TestBuildHLSDiscontinuityDownloadCommand(t *testing.T) {
	dir := t.TempDir()
	toolchain := readyToolchain(dir)
	tempDir := filepath.Join(dir, "fallback-split")
	spec, err := buildHLSDiscontinuityDownloadCommand(
		toolchain,
		DownloadRequest{
			URL:                 "https://chzzk.naver.com/video/15461111",
			OutputDir:           filepath.Join(dir, "downloads"),
			ConcurrentFragments: 4,
		},
		tempDir,
		"all[format_id~='^1080p(?:-[0-9]+)?$']",
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, flag := range []string{"--hls-split-discontinuity", "--no-continue"} {
		if !slices.Contains(spec.Args, flag) {
			t.Fatalf("missing split flag %q: %#v", flag, spec.Args)
		}
	}
	for _, pair := range [][2]string{
		{"--downloader", "m3u8:native"},
		{"--concurrent-fragments", "4"},
		{"--paths", tempDir},
		{"--paths", "temp:" + tempDir},
	} {
		if !hasArgumentPair(spec.Args, pair[0], pair[1]) {
			t.Fatalf("missing split argument pair %#v in %#v", pair, spec.Args)
		}
	}
}

func TestParseHLSSplitFileLineMapsDiscontinuityIndex(t *testing.T) {
	formats := []hlsFallbackFormat{{FormatID: "1080p", Ext: "mp4"}}
	for _, testCase := range []struct {
		formatID string
		index    int
	}{
		{formatID: "1080p-0", index: 0},
		{formatID: "1080p-3", index: 3},
	} {
		file, ok := parseHLSSplitFileLine(
			hlsSplitFilePrefix+testCase.formatID+"\tC:\\video\\part.mp4",
			formats,
		)
		if !ok {
			t.Fatalf("expected split file line for %q", testCase.formatID)
		}
		if file.Index != testCase.index || file.BaseFormat.FormatID != "1080p" {
			t.Fatalf("unexpected split file for %q: %#v", testCase.formatID, file)
		}
	}
}

func TestValidateHLSSplitFilesRejectsMissingSegment(t *testing.T) {
	format := hlsFallbackFormat{FormatID: "1080p"}
	err := validateHLSSplitFiles(
		[]hlsFallbackFormat{format},
		[]hlsSplitFile{
			{BaseFormat: format, FormatID: "1080p-0", Index: 0, Path: "0.mp4"},
			{BaseFormat: format, FormatID: "1080p-2", Index: 2, Path: "2.mp4"},
		},
	)
	if err == nil {
		t.Fatal("expected missing split segment error")
	}
}
