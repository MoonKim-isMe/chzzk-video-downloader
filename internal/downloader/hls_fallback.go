package downloader

import (
	"context"
	"encoding/json"
	"errors"
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
	hlsSplitFormatProbePrefix     = "__CHZZK_HLS_SPLIT_FORMATS__"
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

type hlsSplitCandidate struct {
	BaseFormat hlsFallbackFormat
	Format     hlsFallbackFormat
	Index      int
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

	splitProbeSpec, err := buildHLSSplitFormatProbeCommand(toolchain, request)
	if err != nil {
		return DownloadResult{DiagnosticLines: probeLines}, probeSpec, err
	}
	candidates, splitProbeLines, err := probeHLSSplitCandidates(
		ctx,
		splitProbeSpec,
		formats,
		rawRun,
	)
	probeLines = appendDiagnosticLines(probeLines, splitProbeLines...)
	if err != nil {
		return DownloadResult{DiagnosticLines: probeLines}, splitProbeSpec, err
	}

	runID := "split-" + strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
	splitDir, err := fallbackTemporaryDownloadDirForRequest(request, runID)
	if err != nil {
		return DownloadResult{DiagnosticLines: probeLines}, splitProbeSpec, err
	}
	if err := os.MkdirAll(splitDir, 0o755); err != nil {
		return DownloadResult{DiagnosticLines: probeLines}, splitProbeSpec, fmt.Errorf(
			"구간 분할 임시 폴더를 만들 수 없습니다: %w",
			err,
		)
	}

	result := DownloadResult{DiagnosticLines: probeLines}
	files := make([]hlsSplitFile, 0, len(candidates))
	emptyCandidates := make([]hlsSplitCandidate, 0, 2)
	var usedSpec CommandSpec
	var completedBytes int64

	for candidateIndex, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return result, usedSpec, err
		}

		segmentSpec, err := buildHLSDiscontinuitySegmentDownloadCommand(
			toolchain,
			request,
			splitDir,
			candidate.Format.FormatID,
		)
		if err != nil {
			return result, usedSpec, err
		}
		usedSpec = segmentSpec

		segmentResult, segmentErr := run(
			ctx,
			segmentSpec,
			func(progress DownloadProgress) {
				if progress.Status == "completed" || handler == nil {
					return
				}
				progress.Status = progressStatusFallbackDownloading
				progress.DownloadedBytes += completedBytes
				progress.TotalBytes = 0
				progress.TotalBytesEstimated = false
				progress.ETASeconds = 0
				if len(candidates) > 0 {
					progress.Percent = (
						float64(candidateIndex) + progress.Percent/100
					) / float64(len(candidates)) * 100
				}
				handler(progress)
			},
			nil,
		)
		result.DiagnosticLines = appendDiagnosticLines(
			result.DiagnosticLines,
			segmentResult.DiagnosticLines...,
		)

		if segmentErr != nil {
			if isEmptyHLSDownloadError(segmentErr) {
				emptyCandidates = append(emptyCandidates, candidate)
				result.DiagnosticLines = appendDiagnosticLine(
					result.DiagnosticLines,
					OutputLine{
						Stream: StreamStderr,
						Text: fmt.Sprintf(
							"discontinuity split: empty segment candidate %s",
							candidate.Format.FormatID,
						),
					},
				)
				continue
			}
			return result, segmentSpec, fmt.Errorf(
				"HLS 구간 %s 다운로드에 실패했습니다: %w",
				candidate.Format.FormatID,
				segmentErr,
			)
		}

		if strings.TrimSpace(segmentResult.FinalPath) == "" {
			return result, segmentSpec, fmt.Errorf(
				"HLS 구간 %s의 완료 파일 경로를 확인할 수 없습니다",
				candidate.Format.FormatID,
			)
		}
		info, statErr := os.Stat(segmentResult.FinalPath)
		if statErr != nil {
			return result, segmentSpec, fmt.Errorf(
				"HLS 구간 %s 완료 파일을 확인할 수 없습니다: %w",
				candidate.Format.FormatID,
				statErr,
			)
		}
		if info.Size() <= 0 {
			emptyCandidates = append(emptyCandidates, candidate)
			result.DiagnosticLines = appendDiagnosticLine(
				result.DiagnosticLines,
				OutputLine{
					Stream: StreamStderr,
					Text: fmt.Sprintf(
						"discontinuity split: zero-byte segment candidate %s",
						candidate.Format.FormatID,
					),
				},
			)
			continue
		}

		files = append(files, hlsSplitFile{
			BaseFormat: candidate.BaseFormat,
			FormatID:   candidate.Format.FormatID,
			Index:      candidate.Index,
			Path:       segmentResult.FinalPath,
		})
		completedBytes += info.Size()
		result.LastProgress = segmentResult.LastProgress
		result.LastProgress.Status = progressStatusFallbackDownloading
		result.LastProgress.DownloadedBytes = completedBytes
		result.LastProgress.TotalBytes = 0
		result.LastProgress.TotalBytesEstimated = false
		result.LastProgress.ETASeconds = 0
	}

	if err := validateHLSSplitCandidateResults(
		formats,
		candidates,
		files,
		emptyCandidates,
	); err != nil {
		return result, usedSpec, err
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
	result.DiagnosticLines = appendDiagnosticLines(result.DiagnosticLines, assemblyLines...)
	if err != nil {
		return result, usedSpec, err
	}

	promotedPath, err := promoteFallbackDownload(splitDir, request.OutputDir, finalPath)
	if err != nil {
		return result, usedSpec, err
	}
	result.FinalPath = promotedPath
	result.LastProgress.Status = "completed"
	result.LastProgress.Percent = 100
	result.LastProgress.DownloadedBytes = completedBytes
	result.LastProgress.TotalBytes = 0
	result.LastProgress.TotalBytesEstimated = false
	result.LastProgress.ETASeconds = 0
	if handler != nil {
		handler(result.LastProgress)
	}
	return result, usedSpec, nil
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
		"--print", hlsFormatProbeRequestedPrefix + "%(requested_formats.:.{format_id,vcodec,acodec,ext})j",
		"--print", hlsFormatProbeSinglePrefix + "%(.{format_id,vcodec,acodec,ext})j",
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

func buildHLSSplitFormatProbeCommand(
	toolchain ToolchainStatus,
	request DownloadRequest,
) (CommandSpec, error) {
	if !toolchain.YTDLP.Available {
		return CommandSpec{}, fmt.Errorf("영상 다운로드 기능을 사용할 수 없습니다")
	}
	videoURL, err := normalizeVideoURL(request.URL)
	if err != nil {
		return CommandSpec{}, err
	}

	return CommandSpec{
		Path: toolchain.YTDLP.Path,
		Args: []string{
			"--ignore-config",
			"--simulate",
			"--no-playlist",
			"--color", "never",
			"--output-na-placeholder", "",
			"--hls-split-discontinuity",
			"--print",
			hlsSplitFormatProbePrefix + "%(formats.:.{format_id,vcodec,acodec,ext})j",
			videoURL,
		},
	}, nil
}

func probeHLSSplitCandidates(
	ctx context.Context,
	spec CommandSpec,
	baseFormats []hlsFallbackFormat,
	rawRun rawCommandRunner,
) ([]hlsSplitCandidate, []OutputLine, error) {
	var available []hlsFallbackFormat
	var lines []OutputLine

	err := rawRun(ctx, spec, func(line OutputLine) {
		lines = appendDiagnosticLine(lines, line)
		if !strings.HasPrefix(line.Text, hlsSplitFormatProbePrefix) {
			return
		}
		raw := strings.TrimSpace(strings.TrimPrefix(line.Text, hlsSplitFormatProbePrefix))
		if raw == "" {
			return
		}
		var values []hlsFallbackFormat
		if json.Unmarshal([]byte(raw), &values) == nil {
			available = values
		}
	})
	if err != nil {
		return nil, lines, fmt.Errorf("HLS 분할 구간 목록을 확인할 수 없습니다: %w", err)
	}

	candidates := make([]hlsSplitCandidate, 0, len(available))
	seen := make(map[string]struct{})
	for _, base := range baseFormats {
		for _, format := range available {
			format.FormatID = strings.TrimSpace(format.FormatID)
			index, ok := splitFormatIndex(base.FormatID, format.FormatID)
			if !ok || format.FormatID == base.FormatID {
				continue
			}
			key := base.FormatID + ":" + strconv.Itoa(index)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			candidates = append(candidates, hlsSplitCandidate{
				BaseFormat: base,
				Format:     format,
				Index:      index,
			})
		}
	}

	if len(candidates) == 0 {
		return nil, lines, fmt.Errorf("HLS discontinuity 분할 구간을 찾을 수 없습니다")
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		leftBase := baseFormatOrder(baseFormats, candidates[i].BaseFormat.FormatID)
		rightBase := baseFormatOrder(baseFormats, candidates[j].BaseFormat.FormatID)
		if leftBase != rightBase {
			return leftBase < rightBase
		}
		return candidates[i].Index < candidates[j].Index
	})

	for _, base := range baseFormats {
		found := false
		for _, candidate := range candidates {
			if candidate.BaseFormat.FormatID == base.FormatID {
				found = true
				break
			}
		}
		if !found {
			return nil, lines, fmt.Errorf(
				"HLS 분할 구간 포맷 %s를 찾을 수 없습니다",
				base.FormatID,
			)
		}
	}
	return candidates, lines, nil
}

func baseFormatOrder(formats []hlsFallbackFormat, formatID string) int {
	for index, format := range formats {
		if format.FormatID == formatID {
			return index
		}
	}
	return len(formats)
}

func buildHLSDiscontinuitySegmentDownloadCommand(
	toolchain ToolchainStatus,
	request DownloadRequest,
	tempDir string,
	formatID string,
) (CommandSpec, error) {
	videoURL, err := normalizeVideoURL(request.URL)
	if err != nil {
		return CommandSpec{}, err
	}
	tempDir = filepath.Clean(strings.TrimSpace(tempDir))
	if tempDir == "" || tempDir == "." {
		return CommandSpec{}, fmt.Errorf("구간 분할 임시 경로가 필요합니다")
	}
	formatID = strings.TrimSpace(formatID)
	if !safeFallbackFormatIDPattern.MatchString(formatID) {
		return CommandSpec{}, fmt.Errorf("구간 분할에 사용할 수 없는 포맷 ID입니다: %s", formatID)
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
		"--format", formatID,
		"--paths", tempDir,
		"--paths", "temp:" + tempDir,
		"--output", "%(title)s [%(id)s].%(format_id)s.%(ext)s",
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

func isEmptyHLSDownloadError(err error) bool {
	for current := err; current != nil; current = errors.Unwrap(current) {
		if strings.Contains(
			strings.ToLower(current.Error()),
			"downloaded file is empty",
		) {
			return true
		}
	}
	return false
}

func validateHLSSplitCandidateResults(
	baseFormats []hlsFallbackFormat,
	candidates []hlsSplitCandidate,
	files []hlsSplitFile,
	emptyCandidates []hlsSplitCandidate,
) error {
	for _, base := range baseFormats {
		var baseCandidates []hlsSplitCandidate
		for _, candidate := range candidates {
			if candidate.BaseFormat.FormatID == base.FormatID {
				baseCandidates = append(baseCandidates, candidate)
			}
		}
		sort.Slice(baseCandidates, func(i, j int) bool {
			return baseCandidates[i].Index < baseCandidates[j].Index
		})

		for index := 1; index < len(baseCandidates); index++ {
			if baseCandidates[index].Index != baseCandidates[index-1].Index+1 {
				return fmt.Errorf(
					"HLS 분할 구간 목록 %s에 인덱스 누락이 있습니다: %d 다음 %d",
					base.FormatID,
					baseCandidates[index-1].Index,
					baseCandidates[index].Index,
				)
			}
		}

		var successfulIndexes []int
		for _, file := range files {
			if file.BaseFormat.FormatID == base.FormatID {
				successfulIndexes = append(successfulIndexes, file.Index)
			}
		}
		if len(successfulIndexes) == 0 {
			return fmt.Errorf(
				"HLS 분할 구간 %s에서 다운로드 가능한 미디어 구간을 찾지 못했습니다",
				base.FormatID,
			)
		}
		sort.Ints(successfulIndexes)
		firstSuccess := successfulIndexes[0]
		lastSuccess := successfulIndexes[len(successfulIndexes)-1]

		successSet := make(map[int]struct{}, len(successfulIndexes))
		for _, index := range successfulIndexes {
			successSet[index] = struct{}{}
		}
		for index := firstSuccess; index <= lastSuccess; index++ {
			if _, ok := successSet[index]; !ok {
				return fmt.Errorf(
					"HLS 분할 구간 %s의 중간 미디어 구간 %d이 비어 있어 안전하게 합칠 수 없습니다",
					base.FormatID,
					index,
				)
			}
		}

		for _, empty := range emptyCandidates {
			if empty.BaseFormat.FormatID != base.FormatID {
				continue
			}
			if empty.Index >= firstSuccess && empty.Index <= lastSuccess {
				return fmt.Errorf(
					"HLS 분할 구간 %s의 중간 미디어 구간 %d이 비어 있어 안전하게 합칠 수 없습니다",
					base.FormatID,
					empty.Index,
				)
			}
		}
	}
	return nil
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
