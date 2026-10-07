package videorepair

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	InspectionProgressEvent = "video-repair:inspection-progress"
	quickSampleDuration      = 2.0
)

type InspectionProgress struct {
	Mode             InspectionMode `json:"mode"`
	Path             string         `json:"path"`
	Percent          int            `json:"percent"`
	Stage            string         `json:"stage"`
	SampleIndex      int            `json:"sampleIndex,omitempty"`
	SampleCount      int            `json:"sampleCount,omitempty"`
	ProcessedSeconds float64        `json:"processedSeconds,omitempty"`
	Speed            float64        `json:"speed,omitempty"`
	ElapsedSeconds   float64        `json:"elapsedSeconds,omitempty"`
}

type ProgressHandler func(InspectionProgress)

type quickProbeStream struct {
	Index        int    `json:"index"`
	CodecType    string `json:"codec_type"`
	CodecName    string `json:"codec_name"`
	Profile      string `json:"profile"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	AvgFrameRate string `json:"avg_frame_rate"`
	RFrameRate   string `json:"r_frame_rate"`
	Duration     string `json:"duration"`
	StartTime    string `json:"start_time"`
	SampleRate   string `json:"sample_rate"`
	Channels     int    `json:"channels"`
}

type quickProbeResponse struct {
	Streams []quickProbeStream `json:"streams"`
	Format  struct {
		FormatName string `json:"format_name"`
		Duration   string `json:"duration"`
		StartTime  string `json:"start_time"`
	} `json:"format"`
	Error *struct {
		Code   int    `json:"code"`
		String string `json:"string"`
	} `json:"error,omitempty"`
}

type quickSampleResult struct {
	StartSeconds       float64
	EndSeconds         float64
	HadError           bool
	VideoDecodeErrors  int
	AudioDecodeErrors  int
	CorruptFrames      int
	CorruptPackets     int
	PTSErrors          int
	DTSErrors          int
	NonMonotonicDTS    int
	TimestampJumps     int
	DiagnosticOutput   string
}

type commandCapture struct {
	stdout []byte
	stderr []byte
	err    error
}

func QuickInspect(
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
		return InspectionResult{}, fmt.Errorf("빠른 검사 실행 도구 경로가 필요합니다")
	}

	emitInspectionProgress(handler, file.Path, 5, "파일 정보 확인 중", 0, 0)

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
	ffprobeLogPath := writeDiagnosticLog("ffprobe", file.Path, probeCapture.stderr)

	probe, parseErr := parseQuickProbe(probeCapture.stdout)
	if parseErr != nil || probeCapture.err != nil || probe.Error != nil {
		if ffprobeLogPath == "" {
			combinedDiagnostic := append(append([]byte(nil), probeCapture.stderr...), probeCapture.stdout...)
			ffprobeLogPath = writeDiagnosticLog("ffprobe", file.Path, combinedDiagnostic)
		}
		result := failedQuickInspectionResult(file, moovStatus, probeCapture, parseErr)
		result.Diagnostics.FFprobeLogPath = ffprobeLogPath
		if validateErr := result.Validate(); validateErr != nil {
			return InspectionResult{}, validateErr
		}
		emitInspectionProgress(handler, file.Path, 100, "빠른 검사 완료", 0, 0)
		return result, nil
	}

	applyProbeFileInfo(&file, probe)
	emitInspectionProgress(handler, file.Path, 25, "스트림 정보 확인 완료", 0, 0)

	samplePoints := quickSamplePoints(file.DurationSeconds)
	samples := make([]quickSampleResult, 0, len(samplePoints))
	var ffmpegDiagnostics strings.Builder
	for index, start := range samplePoints {
		if err := ctx.Err(); err != nil {
			return InspectionResult{}, err
		}
		percent := 25 + int(math.Round(float64(index)*60/float64(maxInt(len(samplePoints), 1))))
		emitInspectionProgress(
			handler,
			file.Path,
			percent,
			"대표 구간 확인 중",
			index+1,
			len(samplePoints),
		)
		sample := inspectQuickSample(
			ctx,
			ffmpegPath,
			file.Path,
			start,
			quickSampleDuration,
			firstCodec(probe.Streams, "video"),
			firstCodec(probe.Streams, "audio"),
		)
		if errors.Is(ctx.Err(), context.Canceled) {
			return InspectionResult{}, context.Canceled
		}
		samples = append(samples, sample)
		if strings.TrimSpace(sample.DiagnosticOutput) != "" {
			fmt.Fprintf(
				&ffmpegDiagnostics,
				"--- sample %.3f-%.3f ---\n%s\n",
				sample.StartSeconds,
				sample.EndSeconds,
				sample.DiagnosticOutput,
			)
		}
	}
	ffmpegLogPath := writeDiagnosticLog("ffmpeg", file.Path, []byte(ffmpegDiagnostics.String()))

	result := buildQuickInspectionResult(file, probe, moovStatus, samples)
	applyQuickProbeWarnings(&result, string(probeCapture.stderr))
	result.Diagnostics.FFprobeLogPath = ffprobeLogPath
	result.Diagnostics.FFmpegLogPath = ffmpegLogPath
	if err := result.Validate(); err != nil {
		return InspectionResult{}, err
	}
	emitInspectionProgress(handler, file.Path, 100, "빠른 검사 완료", len(samplePoints), len(samplePoints))
	return result, nil
}

func emitInspectionProgress(
	handler ProgressHandler,
	path string,
	percent int,
	stage string,
	sampleIndex int,
	sampleCount int,
) {
	if handler == nil {
		return
	}
	handler(InspectionProgress{
		Mode:        InspectionModeQuick,
		Path:        path,
		Percent:     percent,
		Stage:       stage,
		SampleIndex: sampleIndex,
		SampleCount: sampleCount,
	})
}

func runCapturedCommand(ctx context.Context, program string, args ...string) commandCapture {
	cmd := exec.CommandContext(ctx, program, args...)
	hideConsoleWindow(cmd)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	return commandCapture{stdout: stdout.Bytes(), stderr: stderr.Bytes(), err: err}
}

func parseQuickProbe(output []byte) (quickProbeResponse, error) {
	var payload quickProbeResponse
	if len(bytes.TrimSpace(output)) == 0 {
		return payload, fmt.Errorf("ffprobe 응답이 비어 있습니다")
	}
	if err := json.Unmarshal(output, &payload); err != nil {
		return payload, err
	}
	return payload, nil
}

func applyProbeFileInfo(file *FileInfo, probe quickProbeResponse) {
	if file == nil {
		return
	}
	file.FormatName = strings.TrimSpace(probe.Format.FormatName)
	file.DurationSeconds = parsePositiveFloat(probe.Format.Duration)
	if file.DurationSeconds <= 0 {
		for _, stream := range probe.Streams {
			file.DurationSeconds = math.Max(file.DurationSeconds, parsePositiveFloat(stream.Duration))
		}
	}
	if normalized := normalizeContainer(file.FormatName, file.Extension); normalized != "" {
		file.Container = normalized
		file.ContainerSource = "probe"
	}
	file.MetadataAvailable = true
	file.MetadataError = ""
}

func quickSamplePoints(duration float64) []float64 {
	if duration <= quickSampleDuration*2 {
		return []float64{0}
	}

	candidates := []float64{
		0,
		math.Max(0, duration/2-quickSampleDuration/2),
		math.Max(0, duration-quickSampleDuration-0.5),
	}
	result := make([]float64, 0, len(candidates))
	for _, candidate := range candidates {
		duplicate := false
		for _, existing := range result {
			if math.Abs(existing-candidate) < quickSampleDuration {
				duplicate = true
				break
			}
		}
		if !duplicate {
			result = append(result, candidate)
		}
	}
	return result
}

func inspectQuickSample(
	ctx context.Context,
	ffmpegPath string,
	path string,
	startSeconds float64,
	durationSeconds float64,
	videoCodec string,
	audioCodec string,
) quickSampleResult {
	args := []string{
		"-hide_banner",
		"-nostdin",
		"-v", "warning",
		"-err_detect", "explode",
	}
	if startSeconds > 0 {
		args = append(args, "-ss", formatSeconds(startSeconds))
	}
	args = append(
		args,
		"-i", path,
		"-t", formatSeconds(durationSeconds),
		"-map", "0:v?",
		"-map", "0:a?",
		"-sn",
		"-dn",
		"-f", "null",
		"-",
	)
	capture := runCapturedCommand(ctx, ffmpegPath, args...)
	endSeconds := startSeconds + durationSeconds
	result := analyzeSampleDiagnostics(
		startSeconds,
		endSeconds,
		string(capture.stderr),
		videoCodec,
		audioCodec,
	)
	if capture.err != nil && !errors.Is(capture.err, context.Canceled) {
		result.HadError = true
		if result.VideoDecodeErrors == 0 && result.AudioDecodeErrors == 0 &&
			result.CorruptFrames == 0 && result.CorruptPackets == 0 {
			result.CorruptPackets = 1
		}
	}
	return result
}

func analyzeSampleDiagnostics(
	startSeconds float64,
	endSeconds float64,
	diagnostic string,
	videoCodec string,
	audioCodec string,
) quickSampleResult {
	result := quickSampleResult{
		StartSeconds:     startSeconds,
		EndSeconds:       endSeconds,
		DiagnosticOutput: strings.TrimSpace(diagnostic),
	}
	videoCodec = strings.ToLower(strings.TrimSpace(videoCodec))
	audioCodec = strings.ToLower(strings.TrimSpace(audioCodec))

	for _, rawLine := range strings.Split(diagnostic, "\n") {
		line := strings.ToLower(strings.TrimSpace(rawLine))
		if line == "" {
			continue
		}
		if strings.Contains(line, "non-monoton") && strings.Contains(line, "dts") {
			result.NonMonotonicDTS++
			result.DTSErrors++
			result.HadError = true
		}
		if strings.Contains(line, "timestamp discontinu") ||
			strings.Contains(line, "timestamp jump") ||
			strings.Contains(line, "out of order") {
			result.TimestampJumps++
			result.HadError = true
		}
		if strings.Contains(line, "invalid pts") || strings.Contains(line, "pts has no value") {
			result.PTSErrors++
			result.HadError = true
		}
		if strings.Contains(line, "invalid dts") {
			result.DTSErrors++
			result.HadError = true
		}

		decodeIssue := strings.Contains(line, "error while decoding") ||
			strings.Contains(line, "decode error") ||
			strings.Contains(line, "invalid nal") ||
			strings.Contains(line, "invalid data found") ||
			strings.Contains(line, "concealing ")
		if decodeIssue {
			result.HadError = true
			if videoCodec != "" && strings.Contains(line, "["+videoCodec) {
				result.VideoDecodeErrors++
			} else if audioCodec != "" && strings.Contains(line, "["+audioCodec) {
				result.AudioDecodeErrors++
			} else {
				result.VideoDecodeErrors++
			}
		}
		if strings.Contains(line, "corrupt") || strings.Contains(line, "damaged") {
			result.HadError = true
			if strings.Contains(line, "frame") {
				result.CorruptFrames++
			} else {
				result.CorruptPackets++
			}
		}
	}
	return result
}

func buildQuickInspectionResult(
	file FileInfo,
	probe quickProbeResponse,
	moovStatus MoovAtomStatus,
	samples []quickSampleResult,
) InspectionResult {
	video := buildVideoInspection(probe.Streams)
	audio := buildAudioInspection(probe.Streams)
	timestamps := TimestampInspection{Status: HealthStatusNormal}
	container := ContainerInspection{
		Status:                 HealthStatusNormal,
		Parseable:              true,
		StreamMetadataReadable: len(probe.Streams) > 0,
		IndexStatus:            HealthStatusNormal,
		MoovAtomStatus:         moovStatus,
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

	damageRanges := make([]DamageRange, 0)
	var firstError *float64
	var lastError *float64
	lastHealthy := (*float64)(nil)
	decodeDamage := false
	for _, sample := range samples {
		video.DecodeErrorCount += sample.VideoDecodeErrors
		audio.DecodeErrorCount += sample.AudioDecodeErrors
		video.CorruptFrameCount += sample.CorruptFrames
		video.CorruptPacketCount += sample.CorruptPackets
		timestamps.PTSErrorCount += sample.PTSErrors
		timestamps.DTSErrorCount += sample.DTSErrors
		timestamps.NonMonotonicDTSCount += sample.NonMonotonicDTS
		timestamps.JumpCount += sample.TimestampJumps

		if sample.HadError {
			start := sample.StartSeconds
			end := sample.EndSeconds
			if file.DurationSeconds > 0 {
				end = math.Min(end, file.DurationSeconds)
			}
			if firstError == nil || start < *firstError {
				firstError = floatPointer(start)
			}
			if lastError == nil || end > *lastError {
				lastError = floatPointer(end)
			}
			category := "대표 구간 오류"
			if sample.VideoDecodeErrors+sample.AudioDecodeErrors+sample.CorruptFrames+sample.CorruptPackets > 0 {
				category = "디코딩/손상 데이터"
				decodeDamage = true
			} else if sample.PTSErrors+sample.DTSErrors+sample.NonMonotonicDTS+sample.TimestampJumps > 0 {
				category = "타임스탬프"
			}
			damageRanges = append(damageRanges, DamageRange{
				StartSeconds: start,
				EndSeconds:   end,
				Category:     category,
				ErrorCount:   quickSampleErrorCount(sample),
			})
		} else {
			position := math.Min(file.DurationSeconds, sample.EndSeconds)
			if position > 0 && (lastHealthy == nil || position > *lastHealthy) {
				lastHealthy = floatPointer(position)
			}
		}
	}

	if video.DecodeErrorCount+video.CorruptFrameCount+video.CorruptPacketCount > 0 {
		video.Status = HealthStatusDamaged
	}
	if audio.DecodeErrorCount+audio.CorruptPacketCount > 0 {
		audio.Status = HealthStatusDamaged
	}
	if timestamps.PTSErrorCount+timestamps.DTSErrorCount+timestamps.NonMonotonicDTSCount+timestamps.JumpCount > 0 {
		timestamps.Status = HealthStatusWarning
	}

	avSync := buildAVSyncInspection(probe.Streams)
	status := InspectionStatusNormal
	repairability := RepairabilityNotNeeded
	recommendation := RepairRecommendation{
		Strategy: RepairStrategyNone,
		Summary:  "빠른 검사에서 명확한 손상이 발견되지 않았습니다.",
	}
	summary := "빠른 검사에서 명확한 손상이 발견되지 않았습니다."

	deepInspectionRecommended := false
	deepInspectionReason := ""

	if decodeDamage || container.Status == HealthStatusDamaged {
		status = InspectionStatusDamaged
		repairability = RepairabilityPartial
		recommendation = RepairRecommendation{
			Strategy:    RepairStrategyPartial,
			Summary:     "대표 구간에서 손상 데이터가 감지되었습니다. 정밀 검사 후 손상 구간을 확정하는 것을 권장합니다.",
			QualityLoss: false,
			SegmentLoss: true,
		}
		summary = "대표 구간에서 손상 가능성이 확인되었습니다. 정밀 검사를 권장합니다."
		deepInspectionRecommended = true
		deepInspectionReason = "대표 구간에서 디코딩 또는 손상 데이터 오류가 확인되어 전체 구간 검사가 필요합니다."
	} else if timestamps.Status == HealthStatusWarning {
		status = InspectionStatusWarning
		repairability = RepairabilityLossless
		recommendation = RepairRecommendation{
			Strategy: RepairStrategyTimestampRemux,
			Summary:  "타임스탬프 이상이 감지되었습니다. 정밀 검사 후 무손실 Timestamp 정규화 + Remux를 우선 검토합니다.",
		}
		summary = "타임스탬프 관련 경고가 발견되었습니다."
		deepInspectionRecommended = true
		deepInspectionReason = "대표 구간에서 타임스탬프 경고가 확인되어 전체 구간의 PTS/DTS 상태를 확인하는 것이 좋습니다."
	} else if container.Status == HealthStatusWarning || avSync.Status == HealthStatusWarning {
		status = InspectionStatusWarning
		repairability = RepairabilityNotNeeded
		recommendation = RepairRecommendation{
			Strategy: RepairStrategyNone,
			Summary:  "빠른 검사만으로 복구 필요 여부를 확정하기 어렵습니다. 정밀 검사를 권장합니다.",
		}
		summary = "추가 확인이 필요한 항목이 발견되었습니다."
		deepInspectionRecommended = true
		deepInspectionReason = "컨테이너, 스트림 또는 A/V 동기화 정보에 경고가 있어 정밀 검사를 권장합니다."
	}

	if file.DurationSeconds > 0 && len(samples) > 0 && !samples[len(samples)-1].HadError {
		lastHealthy = floatPointer(file.DurationSeconds)
	}

	return InspectionResult{
		Mode:               InspectionModeQuick,
		Status:             status,
		Repairability:      repairability,
		Summary:            summary,
		InspectedAt:        time.Now().UTC(),
		File:               file,
		Container:          container,
		Video:              video,
		Audio:              audio,
		Timestamps:         timestamps,
		AVSync:             avSync,
		FirstErrorSeconds:  firstError,
		LastErrorSeconds:   lastError,
		LastHealthySeconds: lastHealthy,
		DamageRanges:              mergeDamageRanges(damageRanges),
		Recommendation:            recommendation,
		DeepInspectionRecommended: deepInspectionRecommended,
		DeepInspectionReason:      deepInspectionReason,
	}
}


func applyQuickProbeWarnings(result *InspectionResult, diagnostic string) {
	if result == nil {
		return
	}
	count := nonEmptyLineCount(diagnostic)
	if count == 0 {
		return
	}
	result.Container.ErrorCount += count
	if result.Container.Status == HealthStatusNormal {
		result.Container.Status = HealthStatusWarning
	}
	if result.Status == InspectionStatusNormal {
		result.Status = InspectionStatusWarning
		result.Repairability = RepairabilityNotNeeded
		result.Summary = "영상 정보 확인 과정에서 컨테이너 또는 스트림 경고가 발견되었습니다."
		result.Recommendation = RepairRecommendation{
			Strategy: RepairStrategyNone,
			Summary:  "빠른 검사 경고만으로 복구 방법을 확정하지 않고 정밀 검사를 권장합니다.",
		}
	}
	if result.Status != InspectionStatusFailed {
		result.DeepInspectionRecommended = true
		if strings.TrimSpace(result.DeepInspectionReason) == "" {
			result.DeepInspectionReason = "영상 정보 확인 과정에서 경고가 발생해 전체 스트림 상태를 추가로 확인하는 것이 좋습니다."
		}
	}
}

func failedQuickInspectionResult(
	file FileInfo,
	moovStatus MoovAtomStatus,
	capture commandCapture,
	parseErr error,
) InspectionResult {
	summary := "컨테이너 또는 스트림 정보를 읽지 못해 빠른 검사를 완료하지 못했습니다."
	if moovStatus == MoovAtomMissing {
		summary = "MP4/MOV 컨테이너에서 moov atom을 찾지 못했습니다."
	}
	if parseErr != nil && strings.TrimSpace(string(capture.stderr)) == "" {
		summary = "영상 정보 응답을 해석하지 못해 빠른 검사를 완료하지 못했습니다."
	}
	return InspectionResult{
		Mode:          InspectionModeQuick,
		Status:        InspectionStatusFailed,
		Repairability: RepairabilityImpossible,
		Summary:       summary,
		InspectedAt:   time.Now().UTC(),
		File:          file,
		Container: ContainerInspection{
			Status:                 HealthStatusDamaged,
			Parseable:              false,
			StreamMetadataReadable: false,
			IndexStatus:            HealthStatusUnavailable,
			MoovAtomStatus:         moovStatus,
			ErrorCount:             maxInt(nonEmptyLineCount(string(capture.stderr)), 1),
		},
		Video:      VideoStreamInspection{Status: HealthStatusUnavailable},
		Audio:      AudioStreamInspection{Status: HealthStatusUnavailable},
		Timestamps: TimestampInspection{Status: HealthStatusUnavailable},
		AVSync:     AVSyncInspection{Status: HealthStatusUnavailable},
		DamageRanges: []DamageRange{},
		Recommendation: RepairRecommendation{
			Strategy: RepairStrategyUnavailable,
			Summary:  "표준 FFmpeg 기반 빠른 검사에서 복구 방법을 결정할 수 없습니다. 파일 구조가 심하게 손상되었을 수 있습니다.",
		},
	}
}

func buildVideoInspection(streams []quickProbeStream) VideoStreamInspection {
	for _, stream := range streams {
		if strings.EqualFold(stream.CodecType, "video") {
			fps := parseFrameRate(stream.AvgFrameRate)
			if fps <= 0 {
				fps = parseFrameRate(stream.RFrameRate)
			}
			return VideoStreamInspection{
				Status:          HealthStatusNormal,
				Present:         true,
				Codec:           stream.CodecName,
				Profile:         stream.Profile,
				Width:           stream.Width,
				Height:          stream.Height,
				FPS:             fps,
				DurationSeconds: parsePositiveFloat(stream.Duration),
			}
		}
	}
	return VideoStreamInspection{Status: HealthStatusUnavailable}
}

func buildAudioInspection(streams []quickProbeStream) AudioStreamInspection {
	for _, stream := range streams {
		if strings.EqualFold(stream.CodecType, "audio") {
			sampleRate, _ := strconv.Atoi(strings.TrimSpace(stream.SampleRate))
			return AudioStreamInspection{
				Status:          HealthStatusNormal,
				Present:         true,
				Codec:           stream.CodecName,
				Channels:        stream.Channels,
				SampleRate:      sampleRate,
				DurationSeconds: parsePositiveFloat(stream.Duration),
			}
		}
	}
	return AudioStreamInspection{Status: HealthStatusUnavailable}
}

func buildAVSyncInspection(streams []quickProbeStream) AVSyncInspection {
	var videoDuration, audioDuration, videoStart, audioStart float64
	var hasVideo, hasAudio bool
	for _, stream := range streams {
		switch strings.ToLower(stream.CodecType) {
		case "video":
			if !hasVideo {
				hasVideo = true
				videoDuration = parsePositiveFloat(stream.Duration)
				videoStart = parseFloat(stream.StartTime)
			}
		case "audio":
			if !hasAudio {
				hasAudio = true
				audioDuration = parsePositiveFloat(stream.Duration)
				audioStart = parseFloat(stream.StartTime)
			}
		}
	}
	if !hasVideo || !hasAudio {
		return AVSyncInspection{Status: HealthStatusUnavailable}
	}
	durationDifference := audioDuration - videoDuration
	startDifference := audioStart - videoStart
	status := HealthStatusNormal
	if math.Abs(durationDifference) > 1 || math.Abs(startDifference) > 1 {
		status = HealthStatusWarning
	}
	return AVSyncInspection{
		Status:                    status,
		VideoDurationSeconds:      videoDuration,
		AudioDurationSeconds:      audioDuration,
		DurationDifferenceSeconds: durationDifference,
		StartDifferenceSeconds:    startDifference,
	}
}

func inspectMoovAtom(path string, extension string) MoovAtomStatus {
	if !isISOBaseMediaExtension(extension) {
		return MoovAtomNotApplicable
	}
	file, err := os.Open(path)
	if err != nil {
		return MoovAtomUnknown
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return MoovAtomUnknown
	}
	fileSize := stat.Size()
	var offset int64
	for offset+8 <= fileSize {
		header := make([]byte, 8)
		if _, err := file.ReadAt(header, offset); err != nil {
			return MoovAtomUnknown
		}
		size := int64(binary.BigEndian.Uint32(header[:4]))
		atomType := string(header[4:8])
		headerSize := int64(8)
		if size == 1 {
			extended := make([]byte, 8)
			if _, err := file.ReadAt(extended, offset+8); err != nil {
				return MoovAtomUnknown
			}
			extendedSize := binary.BigEndian.Uint64(extended)
			if extendedSize > uint64(^uint64(0)>>1) {
				return MoovAtomUnknown
			}
			size = int64(extendedSize)
			headerSize = 16
		} else if size == 0 {
			size = fileSize - offset
		}
		if atomType == "moov" {
			return MoovAtomPresent
		}
		if size < headerSize || offset+size > fileSize {
			return MoovAtomUnknown
		}
		offset += size
	}
	return MoovAtomMissing
}

func isISOBaseMediaExtension(extension string) bool {
	switch strings.ToLower(strings.TrimSpace(extension)) {
	case ".mp4", ".m4v", ".mov":
		return true
	default:
		return false
	}
}

func mergeDamageRanges(ranges []DamageRange) []DamageRange {
	if len(ranges) <= 1 {
		return ranges
	}
	merged := make([]DamageRange, 0, len(ranges))
	for _, current := range ranges {
		if len(merged) == 0 {
			merged = append(merged, current)
			continue
		}
		last := &merged[len(merged)-1]
		if current.StartSeconds <= last.EndSeconds+0.5 && current.Category == last.Category {
			last.EndSeconds = math.Max(last.EndSeconds, current.EndSeconds)
			last.ErrorCount += current.ErrorCount
			continue
		}
		merged = append(merged, current)
	}
	return merged
}

func quickSampleErrorCount(sample quickSampleResult) int {
	count := sample.VideoDecodeErrors + sample.AudioDecodeErrors +
		sample.CorruptFrames + sample.CorruptPackets +
		sample.PTSErrors + sample.DTSErrors +
		sample.TimestampJumps
	if count == 0 && sample.HadError {
		return 1
	}
	return count
}

func firstCodec(streams []quickProbeStream, codecType string) string {
	for _, stream := range streams {
		if strings.EqualFold(stream.CodecType, codecType) {
			return stream.CodecName
		}
	}
	return ""
}

func parseFrameRate(value string) float64 {
	value = strings.TrimSpace(value)
	if value == "" || value == "0/0" {
		return 0
	}
	parts := strings.Split(value, "/")
	if len(parts) == 2 {
		numerator := parseFloat(parts[0])
		denominator := parseFloat(parts[1])
		if denominator != 0 {
			return numerator / denominator
		}
	}
	return parseFloat(value)
}

func parsePositiveFloat(value string) float64 {
	parsed := parseFloat(value)
	if parsed > 0 && !math.IsNaN(parsed) && !math.IsInf(parsed, 0) {
		return parsed
	}
	return 0
}

func parseFloat(value string) float64 {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0
	}
	return parsed
}

func formatSeconds(value float64) string {
	return strconv.FormatFloat(math.Max(value, 0), 'f', 3, 64)
}

func nonEmptyLineCount(value string) int {
	count := 0
	for _, line := range strings.Split(value, "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}

func floatPointer(value float64) *float64 {
	return &value
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}

func writeDiagnosticLog(prefix string, sourcePath string, content []byte) string {
	if len(bytes.TrimSpace(content)) == 0 {
		return ""
	}
	directory := filepath.Join(os.TempDir(), "chzzk-video-downloader", "video-repair")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return ""
	}
	base := strings.TrimSuffix(filepath.Base(sourcePath), filepath.Ext(sourcePath))
	base = sanitizeDiagnosticName(base)
	file, err := os.CreateTemp(directory, prefix+"-"+base+"-*.log")
	if err != nil {
		return ""
	}
	path := file.Name()
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return ""
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return ""
	}
	return path
}

func sanitizeDiagnosticName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "video"
	}
	var builder strings.Builder
	for _, char := range value {
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '-' || char == '_' {
			builder.WriteRune(char)
		} else {
			builder.WriteRune('_')
		}
	}
	return builder.String()
}
