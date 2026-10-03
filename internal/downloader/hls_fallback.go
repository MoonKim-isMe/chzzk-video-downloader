package downloader

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	hlsFormatProbeRequestedPrefix = "__CHZZK_HLS_FORMATS__"
	hlsFormatProbeSinglePrefix    = "__CHZZK_HLS_FORMAT__"
	hlsSplitFilePrefix            = "__CHZZK_HLS_SPLIT_FILE__"
)

var safeFallbackFormatIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

type rawCommandRunner func(context.Context, CommandSpec, LineHandler) error

type hlsFallbackFormat struct {
	FormatID string `json:"format_id"`
	VCodec   string `json:"vcodec"`
	ACodec   string `json:"acodec"`
	Ext      string `json:"ext"`
}

type hlsSplitFile struct {
	BaseFormat hlsFallbackFormat
	FormatID   string
	Index      int
	Path       string
}

func executeDiscontinuitySplitFallback(
	ctx context.Context,
	request DownloadRequest,
	toolchain ToolchainStatus,
	handler ProgressHandler,
	run downloadAttemptRunner,
	rawRun rawCommandRunner,
) (DownloadResult, CommandSpec, error) {
	if rawRun == nil {
		return DownloadResult{}, CommandSpec{}, fmt.Errorf("구간 분할 실행기를 사용할 수 없습니다")
	}
	if handler != nil {
		handler(DownloadProgress{Status: progressStatusFallbackPreparing})
	}

	probeSpec, err := buildHLSFormatProbeCommand(toolchain, request)
	if err != nil {
		return DownloadResult{}, probeSpec, err
	}
	formats, probeLines, err := probeHLSSelectedFormats(ctx, probeSpec, rawRun)
	if err != nil {
		return DownloadResult{DiagnosticLines: probeLines}, probeSpec, err
	}

	selector, err := buildDiscontinuityFormatSelector(formats)
	if err != nil {
		return DownloadResult{DiagnosticLines: probeLines}, probeSpec, err
	}

	runID := "split-" + strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
	splitDir, err := fallbackTemporaryDownloadDirForRequest(request, runID)
	if err != nil {
		return DownloadResult{DiagnosticLines: probeLines}, probeSpec, err
	}
	if err := os.MkdirAll(splitDir, 0o755); err != nil {
		return DownloadResult{DiagnosticLines: probeLines}, probeSpec, fmt.Errorf(
			"구간 분할 임시 폴더를 만들 수 없습니다: %w",
			err,
		)
	}

	splitSpec, err := buildHLSDiscontinuityDownloadCommand(toolchain, request, splitDir, selector)
	if err != nil {
		return DownloadResult{DiagnosticLines: probeLines}, probeSpec, err
	}

	files := make([]hlsSplitFile, 0, 8)
	splitResult, splitErr := run(
		ctx,
		splitSpec,
		func(progress DownloadProgress) {
			if progress.Status == "completed" || handler == nil {
				return
			}
			progress.Status = progressStatusFallbackDownloading
			handler(progress)
		},
		func(line OutputLine) {
			if file, ok := parseHLSSplitFileLine(line.Text, formats); ok {
				files = append(files, file)
			}
		},
	)
	splitResult.DiagnosticLines = appendDiagnosticLines(probeLines, splitResult.DiagnosticLines...)
	if splitErr != nil {
		return splitResult, splitSpec, splitErr
	}
	if err := validateHLSSplitFiles(formats, files); err != nil {
		return splitResult, splitSpec, err
	}

	finalPath, assemblyLines, err := assembleHLSSplitFiles(
		ctx,
		toolchain,
		request,
		splitDir,
		formats,
		files,
		rawRun,
	)
	splitResult.DiagnosticLines = appendDiagnosticLines(splitResult.DiagnosticLines, assemblyLines...)
	if err != nil {
		return splitResult, splitSpec, err
	}

	promotedPath, err := promoteFallbackDownload(splitDir, request.OutputDir, finalPath)
	if err != nil {
		return splitResult, splitSpec, err
	}
	splitResult.FinalPath = promotedPath
	splitResult.LastProgress.Status = "completed"
	splitResult.LastProgress.Percent = 100
	if handler != nil {
		handler(splitResult.LastProgress)
	}
	return splitResult, splitSpec, nil
}

func buildHLSFormatProbeCommand(toolchain ToolchainStatus, request DownloadRequest) (CommandSpec, error) {
	if !toolchain.YTDLP.Available {
		return CommandSpec{}, fmt.Errorf("영상 다운로드 기능을 사용할 수 없습니다")
	}
	videoURL, err := normalizeVideoURL(request.URL)
	if err != nil {
		return CommandSpec{}, err
	}
	formatSelector := strings.TrimSpace(request.FormatSelector)
	if formatSelector == "" {
		formatSelector = DefaultFormatSelector
	}

	args := []string{
		"--ignore-config",
		"--simulate",
		"--no-playlist",
		"--color", "never",
		"--output-na-placeholder", "",
		"--format", formatSelector,
		"--print", hlsFormatProbeRequestedPrefix + "%(requested_formats.:.{format_id,vcodec,acodec,ext})#j",
		"--print", hlsFormatProbeSinglePrefix + "%(.{format_id,vcodec,acodec,ext})#j",
		videoURL,
	}
	return CommandSpec{Path: toolchain.YTDLP.Path, Args: args}, nil
}

func probeHLSSelectedFormats(
	ctx context.Context,
	spec CommandSpec,
	rawRun rawCommandRunner,
) ([]hlsFallbackFormat, []OutputLine, error) {
	var requested []hlsFallbackFormat
	var single hlsFallbackFormat
	var lines []OutputLine

	err := rawRun(ctx, spec, func(line OutputLine) {
		lines = appendDiagnosticLine(lines, line)
		switch {
		case strings.HasPrefix(line.Text, hlsFormatProbeRequestedPrefix):
			raw := strings.TrimSpace(strings.TrimPrefix(line.Text, hlsFormatProbeRequestedPrefix))
			if raw == "" {
				return
			}
			var values []hlsFallbackFormat
			if json.Unmarshal([]byte(raw), &values) == nil && len(values) > 0 {
				requested = values
			}
		case strings.HasPrefix(line.Text, hlsFormatProbeSinglePrefix):
			raw := strings.TrimSpace(strings.TrimPrefix(line.Text, hlsFormatProbeSinglePrefix))
			if raw == "" {
				return
			}
			var value hlsFallbackFormat
			if json.Unmarshal([]byte(raw), &value) == nil {
				single = value
			}
		}
	})
	if err != nil {
		return nil, lines, fmt.Errorf("HLS 구간 분할용 포맷을 확인할 수 없습니다: %w", err)
	}

	formats := requested
	if len(formats) == 0 && strings.TrimSpace(single.FormatID) != "" {
		formats = []hlsFallbackFormat{single}
	}
	formats = normalizeFallbackFormats(formats)
	if len(formats) == 0 {
		return nil, lines, fmt.Errorf("HLS 구간 분할용 다운로드 포맷을 확인할 수 없습니다")
	}
	if len(formats) > 2 {
		return nil, lines, fmt.Errorf("HLS 구간 분할은 최대 영상/오디오 2개 포맷 조합까지 지원합니다")
	}
	return formats, lines, nil
}

func normalizeFallbackFormats(formats []hlsFallbackFormat) []hlsFallbackFormat {
	result := make([]hlsFallbackFormat, 0, len(formats))
	seen := make(map[string]struct{}, len(formats))
	for _, format := range formats {
		format.FormatID = strings.TrimSpace(format.FormatID)
		format.VCodec = strings.TrimSpace(format.VCodec)
		format.ACodec = strings.TrimSpace(format.ACodec)
		format.Ext = strings.TrimSpace(format.Ext)
		if format.FormatID == "" {
			continue
		}
		if _, ok := seen[format.FormatID]; ok {
			continue
		}
		seen[format.FormatID] = struct{}{}
		result = append(result, format)
	}
	return result
}

func buildDiscontinuityFormatSelector(formats []hlsFallbackFormat) (string, error) {
	selectors := make([]string, 0, len(formats))
	for _, format := range formats {
		if !safeFallbackFormatIDPattern.MatchString(format.FormatID) {
			return "", fmt.Errorf("구간 분할에 사용할 수 없는 포맷 ID입니다: %s", format.FormatID)
		}
		pattern := regexp.QuoteMeta(format.FormatID)
		selectors = append(
			selectors,
			fmt.Sprintf("all[format_id~='^%s(?:-[0-9]+)?$']", pattern),
		)
	}
	if len(selectors) == 0 {
		return "", fmt.Errorf("구간 분할 다운로드 포맷이 없습니다")
	}
	return strings.Join(selectors, ","), nil
}

func buildHLSDiscontinuityDownloadCommand(
	toolchain ToolchainStatus,
	request DownloadRequest,
	tempDir string,
	formatSelector string,
) (CommandSpec, error) {
	videoURL, err := normalizeVideoURL(request.URL)
	if err != nil {
		return CommandSpec{}, err
	}
	tempDir = filepath.Clean(strings.TrimSpace(tempDir))
	if tempDir == "" || tempDir == "." {
		return CommandSpec{}, fmt.Errorf("구간 분할 임시 경로가 필요합니다")
	}
	concurrentFragments, err := normalizeConcurrentFragments(request.ConcurrentFragments)
	if err != nil {
		return CommandSpec{}, err
	}

	args := []string{
		"--ignore-config",
		"--no-simulate",
		"--progress",
		"--newline",
		"--color", "never",
		"--no-playlist",
		"--windows-filenames",
		"--no-overwrites",
		"--no-continue",
		"--no-keep-fragments",
		"--concurrent-fragments", strconv.Itoa(concurrentFragments),
		"--progress-delta", "0.5",
		"--progress-template", "download:" + progressTemplate,
		"--hls-split-discontinuity",
		"--downloader", "m3u8:native",
		"--format", formatSelector,
		"--paths", tempDir,
		"--paths", "temp:" + tempDir,
		"--output", "%(title)s [%(id)s].%(format_id)s.%(ext)s",
		"--print", "after_move:" + hlsSplitFilePrefix + "%(format_id)s\t%(filepath)s",
		"--print", "after_move:" + finalPathPrefix + "%(filepath)s",
	}
	location, err := ffmpegLocation(toolchain)
	if err != nil {
		return CommandSpec{}, err
	}
	if location != "" {
		args = append(args, "--ffmpeg-location", location)
	}
	args = append(args, videoURL)
	return CommandSpec{Path: toolchain.YTDLP.Path, Args: args}, nil
}

func parseHLSSplitFileLine(line string, formats []hlsFallbackFormat) (hlsSplitFile, bool) {
	if !strings.HasPrefix(line, hlsSplitFilePrefix) {
		return hlsSplitFile{}, false
	}
	payload := strings.TrimPrefix(line, hlsSplitFilePrefix)
	parts := strings.SplitN(payload, "\t", 2)
	if len(parts) != 2 {
		return hlsSplitFile{}, false
	}
	formatID := strings.TrimSpace(parts[0])
	path := strings.TrimSpace(parts[1])
	if formatID == "" || path == "" {
		return hlsSplitFile{}, false
	}

	for _, format := range formats {
		index, ok := splitFormatIndex(format.FormatID, formatID)
		if !ok {
			continue
		}
		return hlsSplitFile{
			BaseFormat: format,
			FormatID:   formatID,
			Index:      index,
			Path:       path,
		}, true
	}
	return hlsSplitFile{}, false
}

func splitFormatIndex(baseID, formatID string) (int, bool) {
	if formatID == baseID {
		return 0, true
	}
	prefix := baseID + "-"
	if !strings.HasPrefix(formatID, prefix) {
		return 0, false
	}
	index, err := strconv.Atoi(strings.TrimPrefix(formatID, prefix))
	if err != nil || index < 0 {
		return 0, false
	}
	return index, true
}

func validateHLSSplitFiles(formats []hlsFallbackFormat, files []hlsSplitFile) error {
	if len(files) == 0 {
		return fmt.Errorf("HLS 구간 분할 다운로드 결과 파일을 찾을 수 없습니다")
	}
	for _, format := range formats {
		var indexes []int
		for _, file := range files {
			if file.BaseFormat.FormatID == format.FormatID {
				indexes = append(indexes, file.Index)
			}
		}
		if len(indexes) == 0 {
			return fmt.Errorf("HLS 구간 분할 포맷 %s의 결과가 없습니다", format.FormatID)
		}
		sort.Ints(indexes)
		for expected, actual := range indexes {
			if actual != expected {
				return fmt.Errorf(
					"HLS 구간 분할 포맷 %s의 구간이 누락되었습니다: expected=%d actual=%d",
					format.FormatID,
					expected,
					actual,
				)
			}
		}
	}
	return nil
}

func assembleHLSSplitFiles(
	ctx context.Context,
	toolchain ToolchainStatus,
	request DownloadRequest,
	splitDir string,
	formats []hlsFallbackFormat,
	files []hlsSplitFile,
	rawRun rawCommandRunner,
) (string, []OutputLine, error) {
	var diagnostics []OutputLine
	grouped := make(map[string][]hlsSplitFile, len(formats))
	for _, file := range files {
		grouped[file.BaseFormat.FormatID] = append(grouped[file.BaseFormat.FormatID], file)
	}
	for key := range grouped {
		sort.Slice(grouped[key], func(i, j int) bool {
			return grouped[key][i].Index < grouped[key][j].Index
		})
	}

	if len(formats) == 1 {
		group := grouped[formats[0].FormatID]
		finalTarget, err := hlsSplitFinalTarget(splitDir, request, group[0])
		if err != nil {
			return "", diagnostics, err
		}
		err = concatHLSSplitGroup(
			ctx,
			toolchain,
			group,
			finalTarget,
			rawRun,
			&diagnostics,
		)
		return finalTarget, diagnostics, err
	}

	var videoFormat *hlsFallbackFormat
	var audioFormat *hlsFallbackFormat
	for index := range formats {
		format := &formats[index]
		if format.VCodec != "" && !strings.EqualFold(format.VCodec, "none") && videoFormat == nil {
			videoFormat = format
		}
		if (format.VCodec == "" || strings.EqualFold(format.VCodec, "none")) &&
			format.ACodec != "" && !strings.EqualFold(format.ACodec, "none") &&
			audioFormat == nil {
			audioFormat = format
		}
	}
	if videoFormat == nil || audioFormat == nil {
		return "", diagnostics, fmt.Errorf("분할된 영상/오디오 포맷 조합을 확인할 수 없습니다")
	}

	videoGroup := grouped[videoFormat.FormatID]
	audioGroup := grouped[audioFormat.FormatID]
	finalTarget, err := hlsSplitFinalTarget(splitDir, request, videoGroup[0])
	if err != nil {
		return "", diagnostics, err
	}

	videoExt := splitGroupExtension(videoGroup, videoFormat.Ext)
	audioExt := splitGroupExtension(audioGroup, audioFormat.Ext)
	videoPath := filepath.Join(splitDir, "assembled-video."+videoExt)
	audioPath := filepath.Join(splitDir, "assembled-audio."+audioExt)

	if err := concatHLSSplitGroup(
		ctx,
		toolchain,
		videoGroup,
		videoPath,
		rawRun,
		&diagnostics,
	); err != nil {
		return "", diagnostics, err
	}
	if err := concatHLSSplitGroup(
		ctx,
		toolchain,
		audioGroup,
		audioPath,
		rawRun,
		&diagnostics,
	); err != nil {
		return "", diagnostics, err
	}

	mergeSpec := CommandSpec{
		Path: toolchain.FFmpeg.Path,
		Args: []string{
			"-hide_banner",
			"-nostdin",
			"-n",
			"-i", videoPath,
			"-i", audioPath,
			"-map", "0:v:0",
			"-map", "1:a:0",
			"-c", "copy",
			finalTarget,
		},
	}
	if err := runRawWithDiagnostics(ctx, rawRun, mergeSpec, &diagnostics); err != nil {
		return "", diagnostics, fmt.Errorf("분할 HLS 영상/오디오를 합칠 수 없습니다: %w", err)
	}
	return finalTarget, diagnostics, nil
}

func concatHLSSplitGroup(
	ctx context.Context,
	toolchain ToolchainStatus,
	files []hlsSplitFile,
	outputPath string,
	rawRun rawCommandRunner,
	diagnostics *[]OutputLine,
) error {
	if len(files) == 0 {
		return fmt.Errorf("합칠 HLS 구간 파일이 없습니다")
	}
	if len(files) == 1 && strings.EqualFold(filepath.Ext(files[0].Path), filepath.Ext(outputPath)) {
		if err := moveFallbackFile(files[0].Path, outputPath); err != nil {
			return err
		}
		return nil
	}

	listPath := outputPath + ".ffconcat"
	var builder strings.Builder
	builder.WriteString("ffconcat version 1.0\n")
	for _, file := range files {
		builder.WriteString("file '")
		builder.WriteString(escapeFFConcatPath(file.Path))
		builder.WriteString("'\n")
	}
	if err := os.WriteFile(listPath, []byte(builder.String()), 0o644); err != nil {
		return fmt.Errorf("HLS 구간 목록을 만들 수 없습니다: %w", err)
	}

	spec := CommandSpec{
		Path: toolchain.FFmpeg.Path,
		Args: []string{
			"-hide_banner",
			"-nostdin",
			"-n",
			"-f", "concat",
			"-safe", "0",
			"-i", listPath,
			"-c", "copy",
			outputPath,
		},
	}
	if err := runRawWithDiagnostics(ctx, rawRun, spec, diagnostics); err != nil {
		return fmt.Errorf("분할 HLS 구간을 합칠 수 없습니다: %w", err)
	}
	return nil
}

func runRawWithDiagnostics(
	ctx context.Context,
	rawRun rawCommandRunner,
	spec CommandSpec,
	diagnostics *[]OutputLine,
) error {
	return rawRun(ctx, spec, func(line OutputLine) {
		*diagnostics = appendDiagnosticLine(*diagnostics, line)
	})
}

func hlsSplitFinalTarget(
	splitDir string,
	request DownloadRequest,
	first hlsSplitFile,
) (string, error) {
	name := filepath.Base(first.Path)
	ext := strings.TrimPrefix(filepath.Ext(name), ".")
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	stem = strings.TrimSuffix(stem, "."+first.FormatID)
	if strings.TrimSpace(stem) == "" {
		videoURL, err := normalizeVideoURL(request.URL)
		if err != nil {
			return "", err
		}
		videoNo, err := videoNoFromNormalizedURL(videoURL)
		if err != nil {
			return "", err
		}
		stem = "VOD " + strconv.FormatInt(videoNo, 10)
	}
	outputExt, err := normalizeOutputFormat(request.OutputFormat)
	if err != nil {
		return "", err
	}
	if outputExt == "" {
		outputExt = ext
	}
	if outputExt == "" {
		outputExt = "mp4"
	}
	return filepath.Join(splitDir, stem+"."+outputExt), nil
}

func splitGroupExtension(files []hlsSplitFile, fallback string) string {
	if len(files) > 0 {
		if ext := strings.TrimPrefix(filepath.Ext(files[0].Path), "."); ext != "" {
			return ext
		}
	}
	if fallback = strings.TrimPrefix(strings.TrimSpace(fallback), "."); fallback != "" {
		return fallback
	}
	return "mp4"
}

func moveFallbackFile(source, destination string) error {
	if filepath.Clean(source) == filepath.Clean(destination) {
		return nil
	}
	if _, err := os.Stat(destination); err == nil {
		return fmt.Errorf("동일한 임시 결과 파일이 이미 존재합니다: %s", destination)
	} else if !os.IsNotExist(err) {
		return err
	}
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		lastErr = os.Rename(source, destination)
		if lastErr == nil {
			return nil
		}
		if attempt < 4 {
			time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond)
		}
	}
	return fmt.Errorf("HLS 구간 결과 파일을 이동할 수 없습니다: %w", lastErr)
}

func escapeFFConcatPath(path string) string {
	value := filepath.ToSlash(filepath.Clean(path))
	return strings.ReplaceAll(value, "'", "'\\''")
}

func appendDiagnosticLines(lines []OutputLine, extra ...OutputLine) []OutputLine {
	for _, line := range extra {
		lines = appendDiagnosticLine(lines, line)
	}
	return lines
}
