package videorepair

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"
	"time"
)

const RepairProgressEvent = "video-repair:repair-progress"

const maxRepairInt64 int64 = 1<<63 - 1

type RepairPlan struct {
	SourcePath             string         `json:"sourcePath"`
	OutputPath             string         `json:"outputPath"`
	Strategy               RepairStrategy `json:"strategy"`
	InspectionMode         InspectionMode `json:"inspectionMode"`
	SourceSizeBytes        int64          `json:"sourceSizeBytes"`
	SourceModifiedUnixMilli int64          `json:"sourceModifiedUnixMilli"`
	RequiredFreeBytes      int64          `json:"requiredFreeBytes"`
	Summary         string         `json:"summary"`
	ExpectedTime    string         `json:"expectedTime"`
	QualityLoss     bool           `json:"qualityLoss"`
	SegmentLoss     bool           `json:"segmentLoss"`
	Executable      bool           `json:"executable"`
	DurationSeconds float64        `json:"durationSeconds,omitempty"`
	EndSeconds      float64        `json:"endSeconds,omitempty"`
}

type RepairProgress struct {
	SourcePath       string         `json:"sourcePath"`
	OutputPath       string         `json:"outputPath"`
	Strategy         RepairStrategy `json:"strategy"`
	Percent          int            `json:"percent"`
	Stage            string         `json:"stage"`
	ProcessedSeconds float64        `json:"processedSeconds,omitempty"`
	Speed            float64        `json:"speed,omitempty"`
	ElapsedSeconds   float64        `json:"elapsedSeconds,omitempty"`
}

type RepairProgressHandler func(RepairProgress)

type RepairResult struct {
	Plan                RepairPlan        `json:"plan"`
	OutputFile          FileInfo          `json:"outputFile"`
	StartedAt           time.Time         `json:"startedAt"`
	CompletedAt         time.Time         `json:"completedAt"`
	ElapsedSeconds      float64           `json:"elapsedSeconds"`
	RepairLogPath       string            `json:"repairLogPath,omitempty"`
	AutoInspection      *InspectionResult `json:"autoInspection,omitempty"`
	AutoInspectionError string            `json:"autoInspectionError,omitempty"`
}

func BuildRepairPlan(result InspectionResult) (RepairPlan, error) {
	if err := result.Validate(); err != nil {
		return RepairPlan{}, err
	}

	plan := RepairPlan{
		SourcePath:             filepath.Clean(result.File.Path),
		Strategy:               RepairStrategyUnavailable,
		InspectionMode:         result.Mode,
		SourceSizeBytes:        result.File.SizeBytes,
		SourceModifiedUnixMilli: result.File.ModifiedUnixMilli,
		Summary:                "현재 검사 결과로는 복구 방법을 결정할 수 없습니다.",
		Executable:             false,
		DurationSeconds:        result.File.DurationSeconds,
	}
	if strings.TrimSpace(plan.SourcePath) == "" {
		return RepairPlan{}, fmt.Errorf("원본 파일 경로가 필요합니다")
	}

	switch {
	case result.Status == InspectionStatusFailed || result.Repairability == RepairabilityImpossible:
		return plan, nil
	case result.Repairability == RepairabilityNotNeeded || result.Status == InspectionStatusNormal:
		plan.Strategy = RepairStrategyNone
		plan.Summary = "검사 결과 복구가 필요한 문제가 확인되지 않았습니다."
		plan.ExpectedTime = "복구 불필요"
		return plan, nil
	}

	plan.Strategy = chooseRepairStrategy(result)
	plan.QualityLoss = plan.Strategy == RepairStrategyReencode
	plan.SegmentLoss = plan.Strategy == RepairStrategyPartial || plan.Strategy == RepairStrategyTruncate
	plan.EndSeconds = repairEndSeconds(result, plan.Strategy)
	plan.Summary = repairPlanSummary(plan.Strategy, result.Mode)
	plan.ExpectedTime = repairExpectedTime(plan.Strategy)
	plan.Executable = plan.Strategy != RepairStrategyNone && plan.Strategy != RepairStrategyUnavailable

	if !plan.Executable {
		return plan, nil
	}

	outputPath, err := nextRepairOutputPath(plan.SourcePath)
	if err != nil {
		return RepairPlan{}, err
	}
	if sameRepairPath(plan.SourcePath, outputPath) {
		return RepairPlan{}, fmt.Errorf("복구 결과는 원본 파일과 같은 경로를 사용할 수 없습니다")
	}
	plan.OutputPath = outputPath
	plan.RequiredFreeBytes = requiredRepairFreeBytes(plan, plan.SourceSizeBytes)

	if plan.Strategy == RepairStrategyTruncate && plan.EndSeconds <= 0 {
		plan.Strategy = RepairStrategyUnavailable
		plan.Executable = false
		plan.Summary = "정상적으로 보존 가능한 마지막 위치를 확인할 수 없어 부분 복구를 실행할 수 없습니다."
		plan.ExpectedTime = ""
		plan.OutputPath = ""
	}
	return plan, nil
}

func chooseRepairStrategy(result InspectionResult) RepairStrategy {
	if result.Repairability == RepairabilityReencode || result.Recommendation.Strategy == RepairStrategyReencode {
		return RepairStrategyReencode
	}
	if result.Recommendation.Strategy == RepairStrategyTruncate && result.LastHealthySeconds != nil {
		return RepairStrategyTruncate
	}
	if result.Repairability == RepairabilityLossless {
		if result.Recommendation.Strategy == RepairStrategyTimestampRemux ||
			result.Timestamps.Status == HealthStatusWarning ||
			result.AVSync.Status == HealthStatusWarning {
			return RepairStrategyTimestampRemux
		}
		return RepairStrategyRemux
	}
	if result.Repairability == RepairabilityPartial {
		if result.LastHealthySeconds != nil &&
			result.File.DurationSeconds > 0 &&
			*result.LastHealthySeconds < result.File.DurationSeconds*0.98 &&
			(result.Recommendation.Strategy == RepairStrategyTruncate || hasTerminalDamageRange(result)) {
			return RepairStrategyTruncate
		}
		if result.Mode == InspectionModeDeep &&
			(result.Video.DecodeErrorCount > 0 ||
				result.Audio.DecodeErrorCount > 0 ||
				result.Video.CorruptFrameCount > 0) {
			return RepairStrategyReencode
		}
		return RepairStrategyPartial
	}
	return result.Recommendation.Strategy
}

func hasTerminalDamageRange(result InspectionResult) bool {
	if len(result.DamageRanges) == 0 || result.File.DurationSeconds <= 0 {
		return false
	}
	last := result.DamageRanges[len(result.DamageRanges)-1]
	return strings.Contains(last.Category, "디코딩 중단") ||
		last.EndSeconds >= result.File.DurationSeconds-1
}

func repairEndSeconds(result InspectionResult, strategy RepairStrategy) float64 {
	if strategy != RepairStrategyTruncate || result.LastHealthySeconds == nil {
		return 0
	}
	lastHealthy := math.Max(*result.LastHealthySeconds, 0)
	if lastHealthy <= 0 {
		return 0
	}
	safetyMargin := math.Min(0.5, lastHealthy*0.1)
	return math.Max(lastHealthy-safetyMargin, 0.001)
}

func repairPlanSummary(strategy RepairStrategy, mode InspectionMode) string {
	modePrefix := ""
	if mode == InspectionModeQuick {
		modePrefix = "빠른 검사 결과를 기준으로 "
	}
	switch strategy {
	case RepairStrategyRemux:
		return modePrefix + "미디어를 재인코딩하지 않고 새 컨테이너로 다시 구성합니다."
	case RepairStrategyTimestampRemux:
		return modePrefix + "재인코딩 없이 손상 packet을 제외하고 PTS/DTS를 재생성·정규화해 새 컨테이너로 구성합니다."
	case RepairStrategyPartial:
		return modePrefix + "손상 플래그가 있는 packet을 제외하면서 읽을 수 있는 원본 스트림을 최대한 보존합니다."
	case RepairStrategyTruncate:
		return "정밀 검사에서 확인한 마지막 정상 위치 직전까지 안전 여유를 두고 보존해 새 파일을 만듭니다."
	case RepairStrategyReencode:
		return "stream copy로 해결하기 어려운 frame/decode 손상이 있어 읽을 수 있는 영상과 오디오를 재인코딩합니다."
	default:
		return "현재 검사 결과로는 복구 방법을 결정할 수 없습니다."
	}
}

func repairExpectedTime(strategy RepairStrategy) string {
	switch strategy {
	case RepairStrategyRemux, RepairStrategyTimestampRemux, RepairStrategyPartial, RepairStrategyTruncate:
		return "대체로 파일 읽기/쓰기 속도에 가까움"
	case RepairStrategyReencode:
		return "영상 길이·코덱·CPU 성능에 따라 오래 걸릴 수 있음"
	case RepairStrategyNone:
		return "복구 불필요"
	default:
		return ""
	}
}

func nextRepairOutputPath(sourcePath string) (string, error) {
	sourcePath = filepath.Clean(strings.TrimSpace(sourcePath))
	if sourcePath == "" {
		return "", fmt.Errorf("원본 파일 경로가 필요합니다")
	}
	directory := filepath.Dir(sourcePath)
	extension := filepath.Ext(sourcePath)
	name := strings.TrimSuffix(filepath.Base(sourcePath), extension)

	for index := 1; index <= 10000; index++ {
		suffix := ".repaired"
		if index > 1 {
			suffix = fmt.Sprintf(".repaired-%d", index)
		}
		candidate := filepath.Join(directory, name+suffix+extension)
		_, err := os.Stat(candidate)
		if errors.Is(err, os.ErrNotExist) {
			return candidate, nil
		}
		if err != nil {
			return "", fmt.Errorf("복구 출력 경로를 확인할 수 없습니다: %w", err)
		}
	}
	return "", fmt.Errorf("사용 가능한 복구 출력 파일명을 만들 수 없습니다")
}

func sameRepairPath(left string, right string) bool {
	leftAbsolute, leftErr := filepath.Abs(filepath.Clean(left))
	rightAbsolute, rightErr := filepath.Abs(filepath.Clean(right))
	if leftErr != nil || rightErr != nil {
		return filepath.Clean(left) == filepath.Clean(right)
	}
	if goruntime.GOOS == "windows" {
		return strings.EqualFold(leftAbsolute, rightAbsolute)
	}
	return leftAbsolute == rightAbsolute
}

func (plan RepairPlan) Validate() error {
	if strings.TrimSpace(plan.SourcePath) == "" {
		return fmt.Errorf("원본 파일 경로가 필요합니다")
	}
	if !validRepairStrategy(plan.Strategy) {
		return fmt.Errorf("지원하지 않는 복구 전략입니다: %s", plan.Strategy)
	}
	if !plan.Executable || plan.Strategy == RepairStrategyNone || plan.Strategy == RepairStrategyUnavailable {
		return fmt.Errorf("실행할 수 있는 복구 계획이 아닙니다")
	}
	if strings.TrimSpace(plan.OutputPath) == "" {
		return fmt.Errorf("복구 결과 파일 경로가 필요합니다")
	}
	if !IsSupportedPath(plan.SourcePath) || !IsSupportedPath(plan.OutputPath) {
		return fmt.Errorf("지원하지 않는 동영상 형식의 복구 경로입니다")
	}
	if !strings.EqualFold(filepath.Ext(plan.SourcePath), filepath.Ext(plan.OutputPath)) {
		return fmt.Errorf("복구 결과 파일의 확장자는 원본과 같아야 합니다")
	}
	if sameRepairPath(plan.SourcePath, plan.OutputPath) {
		return fmt.Errorf("복구 결과는 원본 파일과 같은 경로를 사용할 수 없습니다")
	}
	if plan.Strategy == RepairStrategyTruncate && plan.EndSeconds <= 0 {
		return fmt.Errorf("부분 복구 종료 위치가 필요합니다")
	}
	return nil
}

func PreflightRepair(plan RepairPlan) error {
	if err := plan.Validate(); err != nil {
		return err
	}

	stat, err := os.Stat(plan.SourcePath)
	if err != nil {
		return fmt.Errorf("원본 파일을 읽을 수 없습니다: %w", err)
	}
	if stat.IsDir() {
		return fmt.Errorf("원본 경로가 동영상 파일이 아닙니다")
	}
	if plan.SourceSizeBytes > 0 && stat.Size() != plan.SourceSizeBytes {
		return fmt.Errorf("검사 이후 원본 파일 크기가 변경되었습니다. 파일을 다시 검사해 주세요")
	}
	if plan.SourceModifiedUnixMilli > 0 && stat.ModTime().UnixMilli() != plan.SourceModifiedUnixMilli {
		return fmt.Errorf("검사 이후 원본 파일이 변경되었습니다. 파일을 다시 검사해 주세요")
	}

	source, err := os.Open(plan.SourcePath)
	if err != nil {
		return fmt.Errorf("원본 파일 읽기 권한이 없거나 다른 프로그램에서 사용할 수 없도록 잠겨 있습니다: %w", err)
	}
	buffer := make([]byte, 1)
	_, readErr := source.Read(buffer)
	closeErr := source.Close()
	if readErr != nil && !errors.Is(readErr, os.ErrClosed) && stat.Size() > 0 {
		return fmt.Errorf("원본 파일을 읽을 수 없습니다: %w", readErr)
	}
	if closeErr != nil {
		return fmt.Errorf("원본 파일 확인을 마칠 수 없습니다: %w", closeErr)
	}

	outputDirectory := filepath.Dir(plan.OutputPath)
	info, err := os.Stat(outputDirectory)
	if err != nil {
		return fmt.Errorf("복구 결과 폴더를 사용할 수 없습니다: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("복구 결과 경로의 상위 경로가 폴더가 아닙니다")
	}

	writeProbe, err := os.CreateTemp(outputDirectory, ".video-repair-write-check-*")
	if err != nil {
		return fmt.Errorf("복구 결과 폴더에 파일을 쓸 권한이 없습니다: %w", err)
	}
	writeProbePath := writeProbe.Name()
	if err := writeProbe.Close(); err != nil {
		_ = os.Remove(writeProbePath)
		return fmt.Errorf("복구 결과 폴더의 쓰기 권한을 확인할 수 없습니다: %w", err)
	}
	_ = os.Remove(writeProbePath)

	availableBytes, err := freeDiskSpace(outputDirectory)
	if err != nil {
		return fmt.Errorf("복구 결과 폴더의 디스크 여유 공간을 확인할 수 없습니다: %w", err)
	}
	requiredBytes := plan.RequiredFreeBytes
	if requiredBytes <= 0 {
		requiredBytes = requiredRepairFreeBytes(plan, stat.Size())
	}
	if err := validateRepairDiskSpace(requiredBytes, availableBytes); err != nil {
		return err
	}
	return nil
}

func validateRepairDiskSpace(requiredBytes int64, availableBytes uint64) error {
	if requiredBytes <= 0 || availableBytes >= uint64(requiredBytes) {
		return nil
	}
	availableForMessage := int64(availableBytes)
	if availableBytes > uint64(maxRepairInt64) {
		availableForMessage = maxRepairInt64
	}
	return fmt.Errorf(
		"디스크 여유 공간이 부족합니다. 최소 %s가 필요하지만 현재 약 %s를 사용할 수 있습니다",
		formatRepairBytes(requiredBytes),
		formatRepairBytes(availableForMessage),
	)
}

func requiredRepairFreeBytes(plan RepairPlan, sourceSize int64) int64 {
	if sourceSize <= 0 {
		return 128 * 1024 * 1024
	}

	estimatedOutput := float64(sourceSize)
	if plan.Strategy == RepairStrategyTruncate && plan.DurationSeconds > 0 && plan.EndSeconds > 0 {
		ratio := math.Min(math.Max(plan.EndSeconds/plan.DurationSeconds, 0.01), 1)
		estimatedOutput *= ratio
	} else if plan.Strategy == RepairStrategyReencode {
		estimatedOutput *= 1.5
	}

	reserve := float64(128 * 1024 * 1024)
	if plan.Strategy == RepairStrategyReencode {
		reserve = float64(256 * 1024 * 1024)
	}
	required := estimatedOutput + reserve
	if required >= float64(maxRepairInt64) {
		return maxRepairInt64
	}
	return int64(math.Ceil(required))
}

func formatRepairBytes(bytes int64) string {
	if bytes <= 0 {
		return "0 B"
	}
	units := []string{"B", "KB", "MB", "GB", "TB"}
	value := float64(bytes)
	index := 0
	for value >= 1024 && index < len(units)-1 {
		value /= 1024
		index++
	}
	if index == 0 {
		return fmt.Sprintf("%d %s", bytes, units[index])
	}
	return fmt.Sprintf("%.1f %s", value, units[index])
}

func Repair(
	ctx context.Context,
	ffprobePath string,
	ffmpegPath string,
	plan RepairPlan,
	handler RepairProgressHandler,
) (RepairResult, error) {
	if err := ctx.Err(); err != nil {
		return RepairResult{}, err
	}
	if err := PreflightRepair(plan); err != nil {
		return RepairResult{}, err
	}
	ffprobePath = strings.TrimSpace(ffprobePath)
	ffmpegPath = strings.TrimSpace(ffmpegPath)
	if ffprobePath == "" || ffmpegPath == "" {
		return RepairResult{}, fmt.Errorf("복구 실행 도구 경로가 필요합니다")
	}

	if _, err := os.Stat(plan.OutputPath); err == nil {
		return RepairResult{}, fmt.Errorf("복구 결과 파일이 이미 존재합니다: %s", plan.OutputPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return RepairResult{}, fmt.Errorf("복구 결과 경로를 확인할 수 없습니다: %w", err)
	}

	tempPath, err := reserveRepairTempPath(plan.OutputPath)
	if err != nil {
		return RepairResult{}, err
	}
	defer func() {
		_ = os.Remove(tempPath)
	}()

	startedAt := time.Now()
	emitRepairProgress(handler, plan, 0, "복구 준비 중", 0, 0, 0)
	logPath, err := runRepairFFmpeg(ctx, ffmpegPath, plan, tempPath, handler, startedAt)
	if err != nil {
		return RepairResult{}, err
	}
	if err := validateRepairOutput(ctx, ffprobePath, tempPath); err != nil {
		return RepairResult{}, fmt.Errorf("복구 결과 검증에 실패했습니다: %w", err)
	}
	if _, err := os.Stat(plan.OutputPath); err == nil {
		return RepairResult{}, fmt.Errorf("복구 완료 전에 같은 이름의 파일이 생성되어 결과를 확정하지 않았습니다")
	} else if !errors.Is(err, os.ErrNotExist) {
		return RepairResult{}, fmt.Errorf("최종 출력 경로를 확인할 수 없습니다: %w", err)
	}

	emitRepairProgress(handler, plan, 96, "검증된 복구 파일 확정 중", planTargetDuration(plan), 0, time.Since(startedAt).Seconds())
	if err := finalizeRepairOutput(tempPath, plan.OutputPath); err != nil {
		return RepairResult{}, err
	}

	outputFile, _ := ProbeFile(ctx, ffprobePath, plan.OutputPath)
	if outputFile.Path == "" {
		outputFile = FileInfo{
			Path:            plan.OutputPath,
			Name:            filepath.Base(plan.OutputPath),
			Extension:       strings.ToLower(filepath.Ext(plan.OutputPath)),
			Container:       containerFromExtension(filepath.Ext(plan.OutputPath)),
			ContainerSource: "extension",
		}
		if stat, statErr := os.Stat(plan.OutputPath); statErr == nil {
			outputFile.SizeBytes = stat.Size()
		}
	}

	emitRepairProgress(handler, plan, 98, "복구 결과 빠른 검사 중", outputFile.DurationSeconds, 0, time.Since(startedAt).Seconds())
	var autoInspection *InspectionResult
	autoInspectionError := ""
	inspection, inspectErr := QuickInspect(ctx, ffprobePath, ffmpegPath, plan.OutputPath, nil)
	if inspectErr != nil {
		autoInspectionError = inspectErr.Error()
	} else {
		autoInspection = &inspection
		outputFile = inspection.File
	}

	completedAt := time.Now()
	emitRepairProgress(handler, plan, 100, "복구 완료", outputFile.DurationSeconds, 0, completedAt.Sub(startedAt).Seconds())
	return RepairResult{
		Plan:                plan,
		OutputFile:          outputFile,
		StartedAt:           startedAt.UTC(),
		CompletedAt:         completedAt.UTC(),
		ElapsedSeconds:      completedAt.Sub(startedAt).Seconds(),
		RepairLogPath:       logPath,
		AutoInspection:      autoInspection,
		AutoInspectionError: autoInspectionError,
	}, nil
}

func runRepairFFmpeg(
	ctx context.Context,
	ffmpegPath string,
	plan RepairPlan,
	tempPath string,
	handler RepairProgressHandler,
	startedAt time.Time,
) (string, error) {
	args, err := buildRepairFFmpegArgs(plan, tempPath)
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
	hideConsoleWindow(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("복구 진행률 파이프를 만들 수 없습니다: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("복구 진단 파이프를 만들 수 없습니다: %w", err)
	}

	logFile, logPath := createStreamingDiagnosticLog("ffmpeg-repair", plan.SourcePath)
	if logFile != nil {
		defer logFile.Close()
	}

	if err := cmd.Start(); err != nil {
		return logPath, fmt.Errorf("복구 프로세스를 시작할 수 없습니다: %w", err)
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
	stderrTail := make([]string, 0, maxDeepDiagnosticTail)
	for line := range lines {
		if line.stream == "progress" {
			if applyDeepProgressLine(&progress, line.text) {
				emitRepairProgress(
					handler,
					plan,
					repairProgressPercent(progress.processedSeconds, planTargetDuration(plan)),
					"복구 파일 생성 중",
					progress.processedSeconds,
					progress.speed,
					time.Since(startedAt).Seconds(),
				)
			}
			continue
		}
		if logFile != nil {
			_, _ = fmt.Fprintln(logFile, line.text)
		}
		stderrTail = appendBoundedTail(stderrTail, line.text, maxDeepDiagnosticTail)
	}

	var scanErr error
	for err := range scanErrors {
		if err != nil && scanErr == nil {
			scanErr = err
		}
	}
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return logPath, context.Canceled
	}
	if scanErr != nil {
		return logPath, fmt.Errorf("복구 출력을 읽는 중 오류가 발생했습니다: %w", scanErr)
	}
	if waitErr != nil {
		message := strings.Join(stderrTail, "\n")
		if message == "" {
			message = waitErr.Error()
		}
		return logPath, fmt.Errorf("FFmpeg 복구 실행에 실패했습니다: %s", message)
	}
	return logPath, nil
}

func buildRepairFFmpegArgs(plan RepairPlan, outputPath string) ([]string, error) {
	if err := plan.Validate(); err != nil {
		return nil, err
	}
	args := []string{"-hide_banner", "-nostdin", "-y", "-v", "warning", "-stats_period", "0.5"}

	switch plan.Strategy {
	case RepairStrategyRemux:
		args = append(args, "-i", plan.SourcePath, "-map", "0", "-map_metadata", "0", "-c", "copy")
	case RepairStrategyTimestampRemux:
		args = append(args,
			"-fflags", "+genpts+discardcorrupt",
			"-i", plan.SourcePath,
			"-map", "0",
			"-map_metadata", "0",
			"-c", "copy",
			"-avoid_negative_ts", "make_zero",
		)
	case RepairStrategyPartial:
		args = append(args,
			"-fflags", "+genpts+discardcorrupt",
			"-err_detect", "ignore_err",
			"-i", plan.SourcePath,
			"-map", "0",
			"-map_metadata", "0",
			"-c", "copy",
			"-avoid_negative_ts", "make_zero",
		)
	case RepairStrategyTruncate:
		args = append(args,
			"-fflags", "+genpts+discardcorrupt",
			"-err_detect", "ignore_err",
			"-i", plan.SourcePath,
			"-map", "0",
			"-map_metadata", "0",
			"-c", "copy",
			"-avoid_negative_ts", "make_zero",
			"-t", formatSeconds(plan.EndSeconds),
		)
	case RepairStrategyReencode:
		args = append(args,
			"-fflags", "+genpts+discardcorrupt",
			"-err_detect", "ignore_err",
			"-i", plan.SourcePath,
			"-map", "0:v:0?",
			"-map", "0:a:0?",
			"-sn",
			"-dn",
			"-avoid_negative_ts", "make_zero",
		)
		args = append(args, repairReencodeArgs(filepath.Ext(outputPath))...)
	default:
		return nil, fmt.Errorf("지원하지 않는 복구 전략입니다: %s", plan.Strategy)
	}

	if isISOBaseMediaExtension(filepath.Ext(outputPath)) {
		args = append(args, "-movflags", "+faststart")
	}
	args = append(args, "-progress", "pipe:1", "-nostats", outputPath)
	return args, nil
}

func repairReencodeArgs(extension string) []string {
	switch strings.ToLower(extension) {
	case ".webm":
		return []string{"-c:v", "libvpx-vp9", "-crf", "20", "-b:v", "0", "-c:a", "libopus", "-b:a", "160k"}
	case ".avi":
		return []string{"-c:v", "mpeg4", "-q:v", "3", "-c:a", "mp3", "-b:a", "192k"}
	default:
		return []string{"-c:v", "libx264", "-preset", "medium", "-crf", "18", "-c:a", "aac", "-b:a", "192k"}
	}
}

func reserveRepairTempPath(finalPath string) (string, error) {
	directory := filepath.Dir(finalPath)
	extension := filepath.Ext(finalPath)
	base := strings.TrimSuffix(filepath.Base(finalPath), extension)
	for attempt := 0; attempt < 100; attempt++ {
		candidate := filepath.Join(directory, fmt.Sprintf(".%s.repairing-%d-%d%s", base, time.Now().UnixNano(), attempt, extension))
		file, err := os.OpenFile(candidate, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o666)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("임시 복구 파일을 만들 수 없습니다: %w", err)
		}
		if err := file.Close(); err != nil {
			_ = os.Remove(candidate)
			return "", fmt.Errorf("임시 복구 파일을 준비할 수 없습니다: %w", err)
		}
		return candidate, nil
	}
	return "", fmt.Errorf("임시 복구 파일명을 만들 수 없습니다")
}

func finalizeRepairOutput(tempPath string, finalPath string) error {
	if goruntime.GOOS == "windows" {
		if err := os.Rename(tempPath, finalPath); err != nil {
			return fmt.Errorf("복구 파일을 최종 경로로 확정할 수 없습니다: %w", err)
		}
		return nil
	}

	if err := os.Link(tempPath, finalPath); err != nil {
		return fmt.Errorf("복구 파일을 최종 경로로 안전하게 확정할 수 없습니다: %w", err)
	}
	return nil
}

func validateRepairOutput(ctx context.Context, ffprobePath string, path string) error {
	stat, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("결과 파일을 읽을 수 없습니다: %w", err)
	}
	if stat.IsDir() || stat.Size() <= 0 {
		return fmt.Errorf("결과 파일이 비어 있습니다")
	}
	capture := runCapturedCommand(
		ctx,
		ffprobePath,
		"-v", "error",
		"-show_error",
		"-show_format",
		"-show_streams",
		"-of", "json",
		path,
	)
	if ctx.Err() != nil {
		return context.Canceled
	}
	probe, parseErr := parseQuickProbe(capture.stdout)
	if capture.err != nil || parseErr != nil || probe.Error != nil {
		return fmt.Errorf("ffprobe가 복구 결과를 정상적으로 읽지 못했습니다")
	}
	if len(probe.Streams) == 0 {
		return fmt.Errorf("복구 결과에서 미디어 스트림을 찾지 못했습니다")
	}
	return nil
}

func repairProgressPercent(processedSeconds float64, targetDuration float64) int {
	if targetDuration <= 0 {
		return 5
	}
	percent := int(math.Round(processedSeconds / targetDuration * 94))
	if percent < 1 {
		return 1
	}
	if percent > 94 {
		return 94
	}
	return percent
}

func planTargetDuration(plan RepairPlan) float64 {
	if plan.Strategy == RepairStrategyTruncate && plan.EndSeconds > 0 {
		return plan.EndSeconds
	}
	return plan.DurationSeconds
}

func emitRepairProgress(
	handler RepairProgressHandler,
	plan RepairPlan,
	percent int,
	stage string,
	processedSeconds float64,
	speed float64,
	elapsedSeconds float64,
) {
	if handler == nil {
		return
	}
	handler(RepairProgress{
		SourcePath:       plan.SourcePath,
		OutputPath:       plan.OutputPath,
		Strategy:         plan.Strategy,
		Percent:          percent,
		Stage:            stage,
		ProcessedSeconds: processedSeconds,
		Speed:            speed,
		ElapsedSeconds:   elapsedSeconds,
	})
}
