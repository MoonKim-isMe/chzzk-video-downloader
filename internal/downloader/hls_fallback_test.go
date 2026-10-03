package downloader

import (
	"context"
	"errors"
	"os"
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
	for _, pair := range [][2]string{
		{"--print", hlsFormatProbeRequestedPrefix + "%(requested_formats.:.{format_id,vcodec,acodec,ext})j"},
		{"--print", hlsFormatProbeSinglePrefix + "%(.{format_id,vcodec,acodec,ext})j"},
	} {
		if !hasArgumentPair(spec.Args, pair[0], pair[1]) {
			t.Fatalf("compact JSON probe template missing: %#v in %#v", pair, spec.Args)
		}
	}
	for _, arg := range spec.Args {
		if strings.Contains(arg, "#j") {
			t.Fatalf("pretty JSON probe template must not be used: %#v", spec.Args)
		}
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

func TestBuildHLSSplitFormatProbeCommand(t *testing.T) {
	spec, err := buildHLSSplitFormatProbeCommand(
		readyToolchain(t.TempDir()),
		DownloadRequest{URL: "https://chzzk.naver.com/video/15461111"},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, flag := range []string{"--simulate", "--hls-split-discontinuity"} {
		if !slices.Contains(spec.Args, flag) {
			t.Fatalf("missing split probe flag %q: %#v", flag, spec.Args)
		}
	}
	if !hasArgumentPair(
		spec.Args,
		"--print",
		hlsSplitFormatProbePrefix+"%(formats.:.{format_id,vcodec,acodec,ext})j",
	) {
		t.Fatalf("split format probe output missing: %#v", spec.Args)
	}
}

func TestProbeHLSSplitCandidatesFiltersAndSortsBaseSegments(t *testing.T) {
	base := []hlsFallbackFormat{
		{FormatID: "video", VCodec: "avc1", ACodec: "none", Ext: "mp4"},
		{FormatID: "audio", VCodec: "none", ACodec: "mp4a", Ext: "m4a"},
	}
	candidates, _, err := probeHLSSplitCandidates(
		context.Background(),
		CommandSpec{Path: "yt-dlp.exe"},
		base,
		func(_ context.Context, _ CommandSpec, handler LineHandler) error {
			handler(OutputLine{
				Stream: StreamStdout,
				Text: hlsSplitFormatProbePrefix +
					`[{"format_id":"other-0","ext":"mp4"},{"format_id":"video-1","ext":"mp4"},{"format_id":"audio-0","ext":"m4a"},{"format_id":"video-0","ext":"mp4"},{"format_id":"audio-1","ext":"m4a"}]`,
			})
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		got = append(got, candidate.Format.FormatID)
	}
	expected := []string{"video-0", "video-1", "audio-0", "audio-1"}
	if !slices.Equal(got, expected) {
		t.Fatalf("unexpected split candidates: %#v", got)
	}
}

func TestBuildHLSDiscontinuitySegmentDownloadCommandUsesExactFormat(t *testing.T) {
	dir := t.TempDir()
	tempDir := filepath.Join(dir, "fallback-split")
	spec, err := buildHLSDiscontinuitySegmentDownloadCommand(
		readyToolchain(dir),
		DownloadRequest{
			URL:                 "https://chzzk.naver.com/video/15461111",
			OutputDir:           filepath.Join(dir, "downloads"),
			ConcurrentFragments: 4,
		},
		tempDir,
		"hls-8384-1",
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, flag := range []string{"--hls-split-discontinuity", "--no-continue"} {
		if !slices.Contains(spec.Args, flag) {
			t.Fatalf("missing split segment flag %q: %#v", flag, spec.Args)
		}
	}
	for _, pair := range [][2]string{
		{"--format", "hls-8384-1"},
		{"--downloader", "m3u8:native"},
		{"--concurrent-fragments", "4"},
		{"--paths", tempDir},
		{"--paths", "temp:" + tempDir},
	} {
		if !hasArgumentPair(spec.Args, pair[0], pair[1]) {
			t.Fatalf("missing split segment argument pair %#v in %#v", pair, spec.Args)
		}
	}
}

func TestValidateHLSSplitCandidateResultsAllowsEmptyBoundary(t *testing.T) {
	base := hlsFallbackFormat{FormatID: "hls-8384"}
	candidates := []hlsSplitCandidate{
		{BaseFormat: base, Format: hlsFallbackFormat{FormatID: "hls-8384-0"}, Index: 0},
		{BaseFormat: base, Format: hlsFallbackFormat{FormatID: "hls-8384-1"}, Index: 1},
	}
	files := []hlsSplitFile{
		{BaseFormat: base, FormatID: "hls-8384-1", Index: 1, Path: "part-1.mp4"},
	}
	empty := []hlsSplitCandidate{candidates[0]}

	if err := validateHLSSplitCandidateResults(
		[]hlsFallbackFormat{base},
		candidates,
		files,
		empty,
	); err != nil {
		t.Fatalf("boundary empty segment should be allowed: %v", err)
	}
}

func TestValidateHLSSplitCandidateResultsRejectsEmptyMiddleSegment(t *testing.T) {
	base := hlsFallbackFormat{FormatID: "hls-8384"}
	candidates := []hlsSplitCandidate{
		{BaseFormat: base, Format: hlsFallbackFormat{FormatID: "hls-8384-0"}, Index: 0},
		{BaseFormat: base, Format: hlsFallbackFormat{FormatID: "hls-8384-1"}, Index: 1},
		{BaseFormat: base, Format: hlsFallbackFormat{FormatID: "hls-8384-2"}, Index: 2},
	}
	files := []hlsSplitFile{
		{BaseFormat: base, FormatID: "hls-8384-0", Index: 0, Path: "part-0.mp4"},
		{BaseFormat: base, FormatID: "hls-8384-2", Index: 2, Path: "part-2.mp4"},
	}
	empty := []hlsSplitCandidate{candidates[1]}

	err := validateHLSSplitCandidateResults(
		[]hlsFallbackFormat{base},
		candidates,
		files,
		empty,
	)
	if err == nil || !strings.Contains(err.Error(), "중간 미디어 구간 1") {
		t.Fatalf("expected middle empty segment rejection, got: %v", err)
	}
}

func TestExecuteDiscontinuitySplitFallbackSkipsLeadingEmptySegment(t *testing.T) {
	outputDir := t.TempDir()
	request := DownloadRequest{
		URL:       "https://chzzk.naver.com/video/15461111",
		OutputDir: outputDir,
	}
	toolchain := readyToolchain(t.TempDir())
	downloadRuns := 0
	rawRuns := 0

	result, usedSpec, err := executeDiscontinuitySplitFallback(
		context.Background(),
		request,
		toolchain,
		nil,
		func(
			_ context.Context,
			spec CommandSpec,
			_ ProgressHandler,
			_ LineHandler,
		) (DownloadResult, error) {
			downloadRuns++
			formatID := argumentValue(spec.Args, "--format")
			switch formatID {
			case "hls-8384-0":
				return DownloadResult{
					DiagnosticLines: []OutputLine{
						{Stream: StreamStderr, Text: "ERROR: The downloaded file is empty"},
					},
				}, errors.New("yt-dlp failed: ERROR: The downloaded file is empty")
			case "hls-8384-1":
				tempDir := testCommandTempDir(spec.Args)
				if tempDir == "" {
					t.Fatalf("split temp directory missing: %#v", spec.Args)
				}
				path := filepath.Join(tempDir, "sample [15461111].hls-8384-1.mp4")
				if err := os.WriteFile(path, []byte("complete-split"), 0o644); err != nil {
					t.Fatal(err)
				}
				return DownloadResult{
					FinalPath: path,
					LastProgress: DownloadProgress{
						Status:          "completed",
						DownloadedBytes: int64(len("complete-split")),
						Percent:         100,
					},
				}, nil
			default:
				t.Fatalf("unexpected split format: %q", formatID)
				return DownloadResult{}, nil
			}
		},
		func(_ context.Context, spec CommandSpec, handler LineHandler) error {
			rawRuns++
			if slices.Contains(spec.Args, "--hls-split-discontinuity") {
				handler(OutputLine{
					Stream: StreamStdout,
					Text: hlsSplitFormatProbePrefix +
						`[{"format_id":"hls-8384-0","vcodec":"avc1","acodec":"mp4a","ext":"mp4"},{"format_id":"hls-8384-1","vcodec":"avc1","acodec":"mp4a","ext":"mp4"}]`,
				})
				return nil
			}
			handler(OutputLine{
				Stream: StreamStdout,
				Text: hlsFormatProbeSinglePrefix +
					`{"format_id":"hls-8384","vcodec":"avc1","acodec":"mp4a","ext":"mp4"}`,
			})
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if downloadRuns != 2 {
		t.Fatalf("expected two independent split downloads, got %d", downloadRuns)
	}
	if rawRuns != 2 {
		t.Fatalf("expected selected-format + split-format probes, got %d", rawRuns)
	}
	if !hasArgumentPair(usedSpec.Args, "--format", "hls-8384-1") {
		t.Fatalf("unexpected final split command: %#v", usedSpec.Args)
	}

	expected := filepath.Join(outputDir, "sample [15461111].mp4")
	if result.FinalPath != expected {
		t.Fatalf("unexpected promoted split path: %q", result.FinalPath)
	}
	data, readErr := os.ReadFile(expected)
	if readErr != nil || string(data) != "complete-split" {
		t.Fatalf("unexpected promoted split data: %q %v", string(data), readErr)
	}

	var diagnostics strings.Builder
	for _, line := range result.DiagnosticLines {
		diagnostics.WriteString(line.Text)
		diagnostics.WriteByte('\n')
	}
	if !strings.Contains(
		diagnostics.String(),
		"empty segment candidate hls-8384-0",
	) {
		t.Fatalf("empty boundary segment diagnostic missing:\n%s", diagnostics.String())
	}
}

func TestIsEmptyHLSDownloadErrorUnwrapsCause(t *testing.T) {
	err := &DownloadFailure{
		Kind:    DownloadFailureHLSInitializationFragmentOrder,
		Message: "wrapper",
		Cause:   errors.New("ERROR: The downloaded file is empty"),
	}
	if !isEmptyHLSDownloadError(err) {
		t.Fatal("expected empty HLS download error detection")
	}
}

func argumentValue(args []string, name string) string {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == name {
			return args[index+1]
		}
	}
	return ""
}

func testCommandTempDir(args []string) string {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == "--paths" && strings.HasPrefix(args[index+1], "temp:") {
			return strings.TrimPrefix(args[index+1], "temp:")
		}
	}
	return ""
}
