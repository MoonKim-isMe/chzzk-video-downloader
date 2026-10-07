package videorepair

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	deepDamageMergeGapSeconds = 2.0
	maxDeepDamageRanges       = 512
	maxDeepDiagnosticTail     = 32
)

var deepDiagnosticTimePattern = regexp.MustCompile(`(?i)(?:pts_time[:=]|time=)\s*([0-9:.]+)`)

type deepProcessLine struct {
	stream string
	text   string
}

type deepProgressState struct {
	processedSeconds float64
	speed            float64
	ended            bool
}

type deepDecodeSummary struct {
	ProcessedSeconds      float64
	Speed                 float64
	ElapsedSeconds        float64
	Completed             bool
	ProcessFailed         bool
	VideoDecodeErrors     int
	AudioDecodeErrors     int
	CorruptFrames         int
	VideoCorruptPackets   int
	AudioCorruptPackets   int
	UnknownCorruptPackets int
	PTSErrors             int
	DTSErrors             int
	NonMonotonicDTSCount  int
	TimestampJumps        int
	DamageRanges          []DamageRange
}

type deepDiagnosticClassification struct {
	relevant            bool
	category            string
	videoDecodeErrors   int
	audioDecodeErrors   int
	corruptFrames       int
	videoCorruptPackets int
	audioCorruptPackets int
	unknownCorruptPackets int
	ptsErrors           int
	dtsErrors           int
	nonMonotonicDTS     int
	timestampJumps      int
}

type deepDamageAccumulator struct {
	ranges []DamageRange
}

func DeepInspect(
	ctx context.Context,
	ffprobePath string,
	ffmpegPath string,
	path string,
	handler ProgressHandler,
) (InspectionResult, error) {
	if err := ctx.Err(); err != nil {
		return InspectionResult{}, err
	}

	file, err := basicFileInfo(path)
	if err != nil {
		return InspectionResult{}, err
	}
	ffprobePath = strings.TrimSpace(ffprobePath)
	ffmpegPath = strings.TrimSpace(ffmpegPath)
	if ffprobePath == "" || ffmpegPath == "" {
		return InspectionResult{}, fmt.Errorf("정밀 검사 실행 도구 경로가 필요합니다")
	}

	emitDeepProgress(handler, file.Path, 2, "정밀 검사 준비 중", 0, 0, 0)
	moovStatus := inspectMoovAtom(file.Path, file.Extension)
	probeCapture := runCapturedCommand(
		ctx,
		ffprobePath,
		"-v", "warning",
		"-show_error",
		"-show_format",
		"-show_streams",
		"-of", "json",
		file.Path,
	)
	if errors.Is(probeCapture.err, context.Canceled) || ctx.Err() != nil {
		return InspectionResult{}, context.Canceled
	}

	ffprobeLogPath := writeDiagnosticLog("ffprobe-deep", file.Path, probeCapture.stderr)
	probe, parseErr := parseQuickProbe(probeCapture.stdout)
	if parseErr != nil || probeCapture.err != nil || probe.Error != nil {
		if ffprobeLogPath == "" {
			combined := append(append([]byte(nil), probeCapture.stderr...), probeCapture.stdout...)
			ffprobeLogPath = writeDiagnosticLog("ffprobe-deep", file.Path, combined)
		}
		result := failedDeepInspectionResult(file, moovStatus, probeCapture, parseErr)
		result.Diagnostics.FFprobeLogPath = ffprobeLogPath
		if validateErr := result.Validate(); validateErr != nil {
			return InspectionResult{}, validateErr
		}
		emitDeepProgress(handler, file.Path, 100, "정밀 검사 완료", 0, 0, 0)
		return result, nil
	}

	applyProbeFileInfo(&file, probe)
	emitDeepProgress(handler, file.Path, 5, "전체 스트림 디코딩 준비 중", 0, 0, 0)

	decode, ffmpegLogPath, err := runDeepDecode(
		ctx,
		ffmpegPath,
		file,
		probe,
		handler,
	)
	if err != nil {
		return InspectionResult{}, err
	}

	result := buildDeepInspectionResult(
		file,
		probe,
		moovStatus,
		string(probeCapture.stderr),
		decode,
	)
	result.Diagnostics.FFprobeLogPath = ffprobeLogPath
	result.Diagnostics.FFmpegLogPath = ffmpegLogPath
	if err := result.Validate(); err != nil {
		return InspectionResult{}, err
	}

	emitDeepProgress(
		handler,
		file.Path,
		100,
		"정밀 검사 완료",
		result.fileDurationOrDecoded(),
		decode.Speed,
		decode.ElapsedSeconds,
	)
	return result, nil
}

func runDeepDecode(
	ctx context.Context,
	ffmpegPath string,
	file FileInfo,
	probe quickProbeResponse,
	handler ProgressHandler,
) (deepDecodeSummary, string, error) {
	var summary deepDecodeSummary
	if err := ctx.Err(); err != nil {
		return summary, "", err
	}

	cmd := exec.CommandContext(
		ctx,
		ffmpegPath,
		"-hide_banner",
		"-nostdin",
		"-v", "warning",
		"-stats_period", "0.5",
		"-err_detect", "ignore_err",
		"-i", file.Path,
		"-map", "0:v?",
		"-map", "0:a?",
		"-sn",
		"-dn",
		"-progress", "pipe:1",
		"-nostats",
		"-f", "null",
		"-",
	)
	hideConsoleWindow(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return summary, "", fmt.Errorf("정밀 검사 progress 파이프를 만들 수 없습니다: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return summary, "", fmt.Errorf("정밀 검사 진단 파이프를 만들 수 없습니다: %w", err)
	}

	logFile, logPath := createStreamingDiagnosticLog("ffmpeg-deep", file.Path)
	if logFile != nil {
		defer logFile.Close()
	}

	startedAt := time.Now()
	if err := cmd.Start(); err != nil {
		return summary, logPath, fmt.Errorf("정밀 검사 프로세스를 시작할 수 없습니다: %w", err)
	}

	lines := make(chan deepProcessLine, 256)
	scanErrors := make(chan error, 2)
	var waitGroup sync.WaitGroup
	waitGroup.Add(2)
	go scanDeepProcessStream(stdout, "progress", lines, scanErrors, &waitGroup)
	go scanDeepProcessStream(stderr, "diagnostic", lines, scanErrors, &waitGroup)
	go func() {
		waitGroup.Wait()
		close(lines)
		close(scanErrors)
	}()

	progress := deepProgressState{}
	damage := deepDamageAccumulator{}
	videoIndex := firstStreamIndex(probe.Streams, "video")
	audioIndex := firstStreamIndex(probe.Streams, "audio")
	videoCodec := firstCodec(probe.Streams, "video")
	audioCodec := firstCodec(probe.Streams, "audio")
	stderrTail := make([]string, 0, maxDeepDiagnosticTail)

	for line := range lines {
		if line.stream == "progress" {
			if applyDeepProgressLine(&progress, line.text) {
				summary.ProcessedSeconds = progress.processedSeconds
				summary.Speed = progress.speed
				summary.ElapsedSeconds = time.Since(startedAt).Seconds()
				percent := deepProgressPercent(progress.processedSeconds, file.DurationSeconds)
				emitDeepProgress(
					handler,
					file.Path,
					percent,
					"전체 스트림 디코딩 중",
					progress.processedSeconds,
					progress.speed,
					summary.ElapsedSeconds,
				)
			}
			continue
		}

		if logFile != nil {
			_, _ = fmt.Fprintln(logFile, line.text)
		}
		stderrTail = appendBoundedTail(stderrTail, line.text, maxDeepDiagnosticTail)

		classification := classifyDeepDiagnostic(
			line.text,
			videoIndex,
			audioIndex,
			videoCodec,
			audioCodec,
		)
		if !classification.relevant {
			continue
		}

		summary.VideoDecodeErrors += classification.videoDecodeErrors
		summary.AudioDecodeErrors += classification.audioDecodeErrors
		summary.CorruptFrames += classification.corruptFrames
		summary.VideoCorruptPackets += classification.videoCorruptPackets
		summary.AudioCorruptPackets += classification.audioCorruptPackets
		summary.UnknownCorruptPackets += classification.unknownCorruptPackets
		summary.PTSErrors += classification.ptsErrors
		summary.DTSErrors += classification.dtsErrors
		summary.NonMonotonicDTSCount += classification.nonMonotonicDTS
		summary.TimestampJumps += classification.timestampJumps

		position := progress.processedSeconds
		if explicit, ok := extractDeepDiagnosticTime(line.text); ok {
			position = explicit
		}
		damage.add(position, classification.category, deepClassificationCount(classification))
	}

	var scanErr error
	for err := range scanErrors {
		if err != nil && scanErr == nil {
			scanErr = err
		}
	}

	waitErr := cmd.Wait()
	summary.ElapsedSeconds = time.Since(startedAt).Seconds()
	summary.ProcessedSeconds = math.Max(summary.ProcessedSeconds, progress.processedSeconds)
	summary.Speed = progress.speed
	summary.Completed = deepDecodeCompleted(
		summary.ProcessedSeconds,
		file.DurationSeconds,
		progress.ended,
		waitErr,
	)
	summary.ProcessFailed = waitErr != nil || !summary.Completed
	summary.DamageRanges = damage.ranges

	if ctx.Err() != nil {
		return summary, logPath, context.Canceled
	}
	if scanErr != nil {
		return summary, logPath, fmt.Errorf("정밀 검사 출력을 읽는 중 오류가 발생했습니다: %w", scanErr)
	}

	if !summary.Completed && file.DurationSeconds > summary.ProcessedSeconds {
		damage.addRange(DamageRange{
			StartSeconds: summary.ProcessedSeconds,
			EndSeconds:   file.DurationSeconds,
			Category:     "디코딩 중단",
			ErrorCount:   maxInt(len(stderrTail), 1),
		})
		summary.DamageRanges = damage.ranges
	}

	if summary.Completed && file.DurationSeconds > 0 {
		summary.ProcessedSeconds = file.DurationSeconds
	}
	return summary, logPath, nil
}

func scanDeepProcessStream(
	reader io.Reader,
	stream string,
	lines chan<- deepProcessLine,
	errorsChannel chan<- error,
	waitGroup *sync.WaitGroup,
) {
	defer waitGroup.Done()

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		lines <- deepProcessLine{stream: stream, text: scanner.Text()}
	}
	if err := scanner.Err(); err != nil {
		errorsChannel <- err
	}
}

func applyDeepProgressLine(state *deepProgressState, line string) bool {
	if state == nil {
		return false
	}
	key, value, found := strings.Cut(strings.TrimSpace(line), "=")
	if !found {
		return false
	}
	value = strings.TrimSpace(value)

	switch key {
	case "out_time_us", "out_time_ms":
		if microseconds, err := strconv.ParseFloat(value, 64); err == nil && microseconds >= 0 {
			state.processedSeconds = microseconds / 1_000_000
		}
	case "out_time":
		if seconds, ok := parseFFmpegTime(value); ok {
			state.processedSeconds = seconds
		}
	case "speed":
		state.speed = parseFFmpegSpeed(value)
	case "progress":
		state.ended = strings.EqualFold(value, "end")
		return true
	}
	return false
}

func deepDecodeCompleted(
	processedSeconds float64,
	durationSeconds float64,
	progressEnded bool,
	waitErr error,
) bool {
	if waitErr != nil || !progressEnded {
		return false
	}
	if durationSeconds <= 0 {
		return true
	}
	tolerance := durationSeconds * 0.002
	if tolerance < 0.25 {
		tolerance = 0.25
	}
	if tolerance > 5 {
		tolerance = 5
	}
	return processedSeconds+tolerance >= durationSeconds
}

func parseFFmpegSpeed(value string) float64 {
	value = strings.TrimSuffix(strings.TrimSpace(value), "x")
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) || parsed < 0 {
		return 0
	}
	return parsed
}

func parseFFmpegTime(value string) (float64, bool) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) == 1 {
		parsed, err := strconv.ParseFloat(parts[0], 64)
		return parsed, err == nil && parsed >= 0
	}
	if len(parts) != 3 {
		return 0, false
	}
	hours, errHours := strconv.ParseFloat(parts[0], 64)
	minutes, errMinutes := strconv.ParseFloat(parts[1], 64)
	seconds, errSeconds := strconv.ParseFloat(parts[2], 64)
	if errHours != nil || errMinutes != nil || errSeconds != nil {
		return 0, false
	}
	result := hours*3600 + minutes*60 + seconds
	return result, result >= 0
}

func deepProgressPercent(processedSeconds float64, durationSeconds float64) int {
	if durationSeconds <= 0 {
		return 5
	}
	percent := int(math.Round(processedSeconds / durationSeconds * 100))
	if percent < 5 {
		return 5
	}
	if percent > 99 {
		return 99
	}
	return percent
}

func emitDeepProgress(
	handler ProgressHandler,
	path string,
	percent int,
	stage string,
	processedSeconds float64,
	speed float64,
	elapsedSeconds float64,
) {
	if handler == nil {
		return
	}
	handler(InspectionProgress{
		Mode:             InspectionModeDeep,
		Path:             path,
		Percent:          percent,
		Stage:            stage,
		ProcessedSeconds: processedSeconds,
		Speed:            speed,
		ElapsedSeconds:   elapsedSeconds,
	})
}

func classifyDeepDiagnostic(
	line string,
	videoIndex int,
	audioIndex int,
	videoCodec string,
	audioCodec string,
) deepDiagnosticClassification {
	lower := strings.ToLower(strings.TrimSpace(line))
	if lower == "" {
		return deepDiagnosticClassification{}
	}

	result := deepDiagnosticClassification{}
	isVideo := diagnosticMatchesStream(lower, videoIndex, videoCodec, []string{
		"[h264", "[hevc", "[av1", "[libdav1d", "[vp9", "[vp8", "[mpeg4", "video:",
	})
	isAudio := diagnosticMatchesStream(lower, audioIndex, audioCodec, []string{
		"[aac", "[opus", "[vorbis", "[mp3", "[flac", "audio:",
	})

	if (strings.Contains(lower, "non-monoton") || strings.Contains(lower, "non monoton")) &&
		strings.Contains(lower, "dts") {
		result.relevant = true
		result.category = "타임스탬프"
		result.nonMonotonicDTS++
		result.dtsErrors++
	}
	if strings.Contains(lower, "invalid pts") || strings.Contains(lower, "pts has no value") {
		result.relevant = true
		result.category = "타임스탬프"
		result.ptsErrors++
	}
	if strings.Contains(lower, "invalid dts") {
		result.relevant = true
		result.category = "타임스탬프"
		result.dtsErrors++
	}
	if strings.Contains(lower, "timestamp discontinu") ||
		strings.Contains(lower, "timestamp jump") ||
		strings.Contains(lower, "out of order") {
		result.relevant = true
		result.category = "타임스탬프"
		result.timestampJumps++
	}

	decodeIssue := strings.Contains(lower, "error while decoding") ||
		strings.Contains(lower, "decode error") ||
		strings.Contains(lower, "invalid nal") ||
		strings.Contains(lower, "invalid data found") ||
		strings.Contains(lower, "partial file") ||
		strings.Contains(lower, "concealing ")
	if decodeIssue {
		result.relevant = true
		if isAudio && !isVideo {
			result.category = "오디오 디코딩"
			result.audioDecodeErrors++
		} else if isVideo && !isAudio {
			result.category = "영상 디코딩"
			result.videoDecodeErrors++
		} else {
			result.category = "손상 패킷"
			result.unknownCorruptPackets++
		}
	}

	if strings.Contains(lower, "corrupt") || strings.Contains(lower, "damaged") {
		result.relevant = true
		if strings.Contains(lower, "frame") {
			result.category = "손상 프레임"
			result.corruptFrames++
		} else if isAudio && !isVideo {
			result.category = "오디오 손상 패킷"
			result.audioCorruptPackets++
		} else if isVideo && !isAudio {
			result.category = "영상 손상 패킷"
			result.videoCorruptPackets++
		} else {
			result.category = "손상 패킷"
			result.unknownCorruptPackets++
		}
	}

	return result
}

func diagnosticMatchesStream(
	line string,
	streamIndex int,
	codec string,
	markers []string,
) bool {
	if streamIndex >= 0 {
		streamMarkers := []string{
			fmt.Sprintf("stream #0:%d", streamIndex),
			fmt.Sprintf("#0:%d/", streamIndex),
			fmt.Sprintf("#0:%d]", streamIndex),
			fmt.Sprintf("#0:%d ", streamIndex),
		}
		for _, marker := range streamMarkers {
			if strings.Contains(line, marker) {
				return true
			}
		}
	}
	codec = strings.ToLower(strings.TrimSpace(codec))
	if codec != "" &&
		(strings.Contains(line, "["+codec) ||
			strings.Contains(line, "[dec:"+codec) ||
			strings.Contains(line, "/"+codec)) {
		return true
	}
	for _, marker := range markers {
		if strings.Contains(line, marker) {
			return true
		}
	}
	return false
}

func extractDeepDiagnosticTime(line string) (float64, bool) {
	matches := deepDiagnosticTimePattern.FindStringSubmatch(line)
	if len(matches) < 2 {
		return 0, false
	}
	return parseFFmpegTime(matches[1])
}

func deepClassificationCount(classification deepDiagnosticClassification) int {
	count := classification.videoDecodeErrors +
		classification.audioDecodeErrors +
		classification.corruptFrames +
		classification.videoCorruptPackets +
		classification.audioCorruptPackets +
		classification.unknownCorruptPackets +
		classification.ptsErrors +
		classification.dtsErrors +
		classification.timestampJumps
	if count <= 0 && classification.relevant {
		return 1
	}
	return count
}

func (a *deepDamageAccumulator) add(position float64, category string, count int) {
	position = math.Max(position, 0)
	start := math.Max(0, position-0.25)
	end := position + 0.25
	a.addRange(DamageRange{
		StartSeconds: start,
		EndSeconds:   end,
		Category:     category,
		ErrorCount:   maxInt(count, 1),
	})
}

func (a *deepDamageAccumulator) addRange(next DamageRange) {
	if next.EndSeconds < next.StartSeconds {
		next.EndSeconds = next.StartSeconds
	}
	if strings.TrimSpace(next.Category) == "" {
		next.Category = "기타 오류"
	}
	if next.ErrorCount <= 0 {
		next.ErrorCount = 1
	}

	if len(a.ranges) > 0 {
		last := &a.ranges[len(a.ranges)-1]
		if next.Category == last.Category && next.StartSeconds <= last.EndSeconds+deepDamageMergeGapSeconds {
			last.EndSeconds = math.Max(last.EndSeconds, next.EndSeconds)
			last.ErrorCount += next.ErrorCount
			return
		}
	}

	if len(a.ranges) >= maxDeepDamageRanges {
		last := &a.ranges[len(a.ranges)-1]
		last.EndSeconds = math.Max(last.EndSeconds, next.EndSeconds)
		last.ErrorCount += next.ErrorCount
		if last.Category != next.Category {
			last.Category = "복합 오류"
		}
		return
	}
	a.ranges = append(a.ranges, next)
}

func appendBoundedTail(values []string, value string, limit int) []string {
	value = strings.TrimSpace(value)
	if value == "" || limit <= 0 {
		return values
	}
	if len(values) < limit {
		return append(values, value)
	}
	copy(values, values[1:])
	values[len(values)-1] = value
	return values
}

func buildDeepInspectionResult(
	file FileInfo,
	probe quickProbeResponse,
	moovStatus MoovAtomStatus,
	probeDiagnostic string,
	decode deepDecodeSummary,
) InspectionResult {
	video := buildVideoInspection(probe.Streams)
	audio := buildAudioInspection(probe.Streams)
	video.DecodeErrorCount = decode.VideoDecodeErrors
	video.CorruptFrameCount = decode.CorruptFrames
	video.CorruptPacketCount = decode.VideoCorruptPackets
	audio.DecodeErrorCount = decode.AudioDecodeErrors
	audio.CorruptPacketCount = decode.AudioCorruptPackets

	if video.DecodeErrorCount+video.CorruptFrameCount+video.CorruptPacketCount > 0 {
		video.Status = HealthStatusDamaged
	}
	if audio.DecodeErrorCount+audio.CorruptPacketCount > 0 {
		audio.Status = HealthStatusDamaged
	}

	timestamps := TimestampInspection{
		Status:               HealthStatusNormal,
		PTSErrorCount:        decode.PTSErrors,
		DTSErrorCount:        decode.DTSErrors,
		NonMonotonicDTSCount: decode.NonMonotonicDTSCount,
		JumpCount:            decode.TimestampJumps,
	}
	if decode.PTSErrors+decode.DTSErrors+decode.NonMonotonicDTSCount+decode.TimestampJumps > 0 {
		timestamps.Status = HealthStatusWarning
	}

	container := ContainerInspection{
		Status:                 HealthStatusNormal,
		Parseable:              true,
		StreamMetadataReadable: len(probe.Streams) > 0,
		IndexStatus:            HealthStatusNormal,
		MoovAtomStatus:         moovStatus,
		ErrorCount:             nonEmptyLineCount(probeDiagnostic),
	}
	if container.ErrorCount > 0 {
		container.Status = HealthStatusWarning
	}
	if len(probe.Streams) == 0 {
		container.Status = HealthStatusWarning
		container.StreamMetadataReadable = false
		container.ErrorCount++
	}
	if file.DurationSeconds <= 0 {
		container.Status = HealthStatusWarning
		container.IndexStatus = HealthStatusWarning
		container.ErrorCount++
	}
	if moovStatus == MoovAtomMissing {
		container.Status = HealthStatusDamaged
		container.IndexStatus = HealthStatusDamaged
		container.ErrorCount++
	}
	if decode.ProcessFailed {
		container.ErrorCount++
	}

	avSync := buildAVSyncInspection(probe.Streams)
	damageRanges := append([]DamageRange(nil), decode.DamageRanges...)
	if decode.ProcessFailed && decode.ProcessedSeconds > 0 && len(damageRanges) == 0 {
		end := file.DurationSeconds
		if end <= decode.ProcessedSeconds {
			end = decode.ProcessedSeconds + 0.5
		}
		damageRanges = append(damageRanges, DamageRange{
			StartSeconds: decode.ProcessedSeconds,
			EndSeconds:   end,
			Category:     "디코딩 중단",
			ErrorCount:   1,
		})
	}

	var firstError *float64
	var lastError *float64
	if len(damageRanges) > 0 {
		firstError = floatPointer(damageRanges[0].StartSeconds)
		lastError = floatPointer(damageRanges[len(damageRanges)-1].EndSeconds)
	}

	var lastHealthy *float64
	if decode.ProcessedSeconds > 0 {
		position := decode.ProcessedSeconds
		if file.DurationSeconds > 0 {
			position = math.Min(position, file.DurationSeconds)
		}
		if len(damageRanges) > 0 {
			lastRange := damageRanges[len(damageRanges)-1]
			if position <= lastRange.EndSeconds+1 && lastRange.StartSeconds < position {
				position = lastRange.StartSeconds
			}
		}
		lastHealthy = floatPointer(math.Max(position, 0))
	}

	status := InspectionStatusNormal
	repairability := RepairabilityNotNeeded
	summary := "전체 영상과 오디오 스트림을 끝까지 디코딩했으며 명확한 손상이 발견되지 않았습니다."
	recommendation := RepairRecommendation{
		Strategy: RepairStrategyNone,
		Summary:  "정밀 검사에서 복구가 필요한 손상이 발견되지 않았습니다.",
	}

	decodeDamageCount := decode.VideoDecodeErrors + decode.AudioDecodeErrors + decode.CorruptFrames +
		decode.VideoCorruptPackets + decode.AudioCorruptPackets + decode.UnknownCorruptPackets
	timestampErrorCount := decode.PTSErrors + decode.DTSErrors + decode.NonMonotonicDTSCount + decode.TimestampJumps
	unstableTimestamps := decode.NonMonotonicDTSCount > 0 || decode.TimestampJumps > 0

	switch {
	case decode.ProcessFailed && decode.ProcessedSeconds <= 0:
		status = InspectionStatusFailed
		repairability = RepairabilityImpossible
		summary = "전체 디코딩을 시작하지 못했거나 초기에 중단되어 정밀 검사를 완료하지 못했습니다."
		recommendation = RepairRecommendation{
			Strategy: RepairStrategyUnavailable,
			Summary:  "현재 검사 결과만으로 안전한 복구 방법을 결정할 수 없습니다.",
		}
	case unstableTimestamps:
		status = InspectionStatusWarning
		repairability = RepairabilityReencode
		summary = "전체 디코딩 중 DTS 역행 또는 큰 타임스탬프 불연속이 확인되었습니다."
		recommendation = RepairRecommendation{
			Strategy:    RepairStrategyReencode,
			Summary:     "단순 Remux로 남을 수 있는 타임스탬프 불연속이어서 재인코딩으로 시간축을 다시 구성하는 복구를 권장합니다.",
			QualityLoss: true,
		}
	case decodeDamageCount > 0 || decode.ProcessFailed:
		status = InspectionStatusDamaged
		repairability = RepairabilityPartial
		strategy := RepairStrategyPartial
		recommendationSummary := "손상 데이터 일부를 제외하면서 정상 데이터를 최대한 보존하는 복구를 우선 검토합니다."
		if !decode.Completed && file.DurationSeconds > 0 && decode.ProcessedSeconds < file.DurationSeconds*0.98 {
			strategy = RepairStrategyTruncate
			recommendationSummary = "전체 디코딩이 끝까지 진행되지 않았습니다. 마지막 정상 구간까지 보존하는 부분 복구를 우선 검토합니다."
		}
		summary = "전체 디코딩 과정에서 영상 또는 오디오 손상이 확인되었습니다."
		recommendation = RepairRecommendation{
			Strategy:    strategy,
			Summary:     recommendationSummary,
			QualityLoss: false,
			SegmentLoss: true,
		}
	case timestampErrorCount > 0:
		status = InspectionStatusWarning
		repairability = RepairabilityLossless
		summary = "전체 디코딩은 완료했지만 타임스탬프 관련 이상이 확인되었습니다."
		recommendation = RepairRecommendation{
			Strategy: RepairStrategyTimestampRemux,
			Summary:  "재인코딩 없이 타임스탬프를 정규화한 Remux 복구를 우선 권장합니다.",
		}
	case avSync.Status == HealthStatusWarning && math.Abs(avSync.DurationDifferenceSeconds) > 1:
		status = InspectionStatusWarning
		repairability = RepairabilityPartial
		summary = "영상과 오디오의 재생 길이 차이가 크게 확인되었습니다."
		recommendation = RepairRecommendation{
			Strategy:    RepairStrategyTruncate,
			Summary:     "긴 스트림의 후반 구간을 제거해 영상과 오디오가 함께 존재하는 구간까지만 보존하는 부분 복구를 검토합니다.",
			SegmentLoss: true,
		}
	case avSync.Status == HealthStatusWarning:
		status = InspectionStatusWarning
		repairability = RepairabilityLossless
		summary = "영상과 오디오의 시작 시점 차이가 확인되었습니다."
		recommendation = RepairRecommendation{
			Strategy: RepairStrategyTimestampRemux,
			Summary:  "A/V 시작 타임스탬프를 정규화하는 무손실 Remux를 우선 검토합니다.",
		}
	case container.Status == HealthStatusWarning:
		status = InspectionStatusWarning
		repairability = RepairabilityLossless
		summary = "전체 디코딩은 완료했지만 컨테이너 또는 스트림 메타데이터 경고가 확인되었습니다."
		recommendation = RepairRecommendation{
			Strategy: RepairStrategyRemux,
			Summary:  "미디어 스트림을 재인코딩하지 않는 Remux 복구를 우선 검토합니다.",
		}
	case container.Status == HealthStatusDamaged:
		status = InspectionStatusDamaged
		repairability = RepairabilityPartial
		summary = "컨테이너 구조 손상이 확인되었습니다."
		recommendation = RepairRecommendation{
			Strategy:    RepairStrategyPartial,
			Summary:     "읽을 수 있는 스트림 데이터를 기준으로 새 컨테이너를 구성하는 부분 복구를 검토합니다.",
			SegmentLoss: true,
		}
	}

	return InspectionResult{
		Mode:                      InspectionModeDeep,
		Status:                    status,
		Repairability:             repairability,
		Summary:                   summary,
		InspectedAt:               time.Now().UTC(),
		File:                      file,
		Container:                 container,
		Video:                     video,
		Audio:                     audio,
		Timestamps:                timestamps,
		AVSync:                    avSync,
		FirstErrorSeconds:         firstError,
		LastErrorSeconds:          lastError,
		LastHealthySeconds:        lastHealthy,
		DamageRanges:              damageRanges,
		Recommendation:            recommendation,
		DeepInspectionRecommended: false,
	}
}

func failedDeepInspectionResult(
	file FileInfo,
	moovStatus MoovAtomStatus,
	capture commandCapture,
	parseErr error,
) InspectionResult {
	result := failedQuickInspectionResult(file, moovStatus, capture, parseErr)
	result.Mode = InspectionModeDeep
	result.Summary = "컨테이너 또는 스트림 정보를 읽지 못해 정밀 검사를 시작하지 못했습니다."
	result.Recommendation.Summary = "전체 디코딩을 시작할 수 없어 현재 상태에서는 복구 방법을 결정할 수 없습니다."
	result.DeepInspectionRecommended = false
	result.DeepInspectionReason = ""
	return result
}

func firstStreamIndex(streams []quickProbeStream, codecType string) int {
	for _, stream := range streams {
		if strings.EqualFold(stream.CodecType, codecType) {
			return stream.Index
		}
	}
	return -1
}

func createStreamingDiagnosticLog(prefix string, sourcePath string) (*os.File, string) {
	directory := filepath.Join(os.TempDir(), "chzzk-video-downloader", "video-repair")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return nil, ""
	}
	base := strings.TrimSuffix(filepath.Base(sourcePath), filepath.Ext(sourcePath))
	base = sanitizeDiagnosticName(base)
	file, err := os.CreateTemp(directory, prefix+"-"+base+"-*.log")
	if err != nil {
		return nil, ""
	}
	return file, file.Name()
}

func (r InspectionResult) fileDurationOrDecoded() float64 {
	if r.File.DurationSeconds > 0 {
		return r.File.DurationSeconds
	}
	if r.LastHealthySeconds != nil {
		return *r.LastHealthySeconds
	}
	return 0
}
