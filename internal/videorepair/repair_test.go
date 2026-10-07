package videorepair

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildRepairPlanPrioritizesConservativeStrategies(t *testing.T) {
	file := repairPlanFixture(t)
	tests := []struct {
		name     string
		mutate   func(*InspectionResult)
		strategy RepairStrategy
	}{
		{
			name: "remux",
			mutate: func(result *InspectionResult) {
				result.Status = InspectionStatusWarning
				result.Repairability = RepairabilityLossless
				result.Container.Status = HealthStatusWarning
				result.Recommendation.Strategy = RepairStrategyRemux
			},
			strategy: RepairStrategyRemux,
		},
		{
			name: "timestamp",
			mutate: func(result *InspectionResult) {
				result.Status = InspectionStatusWarning
				result.Repairability = RepairabilityLossless
				result.Timestamps.Status = HealthStatusWarning
				result.Timestamps.DTSErrorCount = 1
				result.Recommendation.Strategy = RepairStrategyTimestampRemux
			},
			strategy: RepairStrategyTimestampRemux,
		},
		{
			name: "deep frame damage reencode",
			mutate: func(result *InspectionResult) {
				result.Mode = InspectionModeDeep
				result.Status = InspectionStatusDamaged
				result.Repairability = RepairabilityPartial
				result.Video.Status = HealthStatusDamaged
				result.Video.DecodeErrorCount = 1
				result.Recommendation.Strategy = RepairStrategyPartial
			},
			strategy: RepairStrategyReencode,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := validInspectionFixture()
			result.File = file
			test.mutate(&result)
			plan, err := BuildRepairPlan(result)
			if err != nil {
				t.Fatal(err)
			}
			if plan.Strategy != test.strategy || !plan.Executable {
				t.Fatalf("unexpected plan: %#v", plan)
			}
		})
	}
}

func TestBuildRepairPlanNonMonotonicTimestampUsesNormalizedReencode(t *testing.T) {
	result := validInspectionFixture()
	result.File = repairPlanFixture(t)
	result.Mode = InspectionModeDeep
	result.Status = InspectionStatusWarning
	result.Repairability = RepairabilityReencode
	result.Timestamps.Status = HealthStatusWarning
	result.Timestamps.NonMonotonicDTSCount = 2
	result.Timestamps.JumpCount = 1
	result.Recommendation.Strategy = RepairStrategyReencode

	plan, err := BuildRepairPlan(result)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Strategy != RepairStrategyReencode || !plan.NormalizeTimestamps {
		t.Fatalf("unexpected timestamp repair plan: %#v", plan)
	}
}

func TestBuildRepairPlanAVDurationMismatchUsesShorterStream(t *testing.T) {
	result := validInspectionFixture()
	result.File = repairPlanFixture(t)
	result.Mode = InspectionModeDeep
	result.Status = InspectionStatusWarning
	result.Repairability = RepairabilityPartial
	result.AVSync = AVSyncInspection{
		Status:                    HealthStatusWarning,
		VideoDurationSeconds:      10,
		AudioDurationSeconds:      6,
		DurationDifferenceSeconds: -4,
	}
	result.Recommendation = RepairRecommendation{
		Strategy:    RepairStrategyTruncate,
		Summary:     "함께 존재하는 구간까지만 보존합니다.",
		SegmentLoss: true,
	}

	plan, err := BuildRepairPlan(result)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Strategy != RepairStrategyTruncate || plan.EndSeconds != 5.5 || !plan.SegmentLoss {
		t.Fatalf("unexpected A/V mismatch plan: %#v", plan)
	}
}

func TestBuildRepairPlanTruncateUsesLastHealthyPosition(t *testing.T) {
	result := validInspectionFixture()
	result.File = repairPlanFixture(t)
	result.Status = InspectionStatusDamaged
	result.Repairability = RepairabilityPartial
	result.Recommendation.Strategy = RepairStrategyTruncate
	result.LastHealthySeconds = float64Pointer(70)
	result.DamageRanges = []DamageRange{{StartSeconds: 70, EndSeconds: 120, Category: "디코딩 중단", ErrorCount: 1}}
	plan, err := BuildRepairPlan(result)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Strategy != RepairStrategyTruncate || plan.EndSeconds != 69.5 || !plan.SegmentLoss {
		t.Fatalf("unexpected truncate plan: %#v", plan)
	}
}

func TestNextRepairOutputPathAvoidsCollisions(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "sample.mp4")
	if err := os.WriteFile(source, []byte("source"), 0o644); err != nil {
		t.Fatal(err)
	}
	first, err := nextRepairOutputPath(source)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(first) != "sample.repaired.mp4" {
		t.Fatalf("unexpected first output: %s", first)
	}
	if err := os.WriteFile(first, []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := nextRepairOutputPath(source)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(second) != "sample.repaired-2.mp4" {
		t.Fatalf("unexpected collision output: %s", second)
	}
}

func TestBuildRepairPlanHandlesNotNeededAndImpossible(t *testing.T) {
	file := repairPlanFixture(t)

	normal := validInspectionFixture()
	normal.File = file
	normal.Status = InspectionStatusNormal
	normal.Repairability = RepairabilityNotNeeded
	normal.Recommendation.Strategy = RepairStrategyNone
	normalPlan, err := BuildRepairPlan(normal)
	if err != nil {
		t.Fatal(err)
	}
	if normalPlan.Executable || normalPlan.Strategy != RepairStrategyNone {
		t.Fatalf("unexpected normal plan: %#v", normalPlan)
	}

	failed := validInspectionFixture()
	failed.File = file
	failed.Status = InspectionStatusFailed
	failed.Repairability = RepairabilityImpossible
	failed.Recommendation.Strategy = RepairStrategyUnavailable
	failedPlan, err := BuildRepairPlan(failed)
	if err != nil {
		t.Fatal(err)
	}
	if failedPlan.Executable || failedPlan.Strategy != RepairStrategyUnavailable {
		t.Fatalf("unexpected impossible plan: %#v", failedPlan)
	}
}

func TestRepairPlanRejectsSourceAsOutput(t *testing.T) {
	plan := RepairPlan{
		SourcePath:      "sample.mp4",
		OutputPath:      "sample.mp4",
		Strategy:        RepairStrategyRemux,
		Executable:      true,
		DurationSeconds: 120,
	}
	if err := plan.Validate(); err == nil {
		t.Fatal("expected same source/output rejection")
	}
}

func TestRepairPlanRejectsDifferentOutputExtension(t *testing.T) {
	plan := RepairPlan{
		SourcePath:      "sample.mp4",
		OutputPath:      "sample.repaired.mkv",
		Strategy:        RepairStrategyRemux,
		Executable:      true,
		DurationSeconds: 120,
	}
	if err := plan.Validate(); err == nil {
		t.Fatal("expected output extension rejection")
	}
}

func TestFinalizeRepairOutputDoesNotOverwriteExistingFile(t *testing.T) {
	directory := t.TempDir()
	tempPath := filepath.Join(directory, "temporary.mp4")
	finalPath := filepath.Join(directory, "final.mp4")
	if err := os.WriteFile(tempPath, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(finalPath, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := finalizeRepairOutput(tempPath, finalPath); err == nil {
		t.Fatal("expected existing final output to be protected")
	}
	content, err := os.ReadFile(finalPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "old" {
		t.Fatalf("existing final file was modified: %q", content)
	}
}

func TestFinalizeRepairOutputCreatesFinalFile(t *testing.T) {
	directory := t.TempDir()
	tempPath := filepath.Join(directory, "temporary.mp4")
	finalPath := filepath.Join(directory, "final.mp4")
	if err := os.WriteFile(tempPath, []byte("repaired"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := finalizeRepairOutput(tempPath, finalPath); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(finalPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "repaired" {
		t.Fatalf("unexpected final content: %q", content)
	}
}

func TestPreflightRepairRejectsChangedSource(t *testing.T) {
	file := repairPlanFixture(t)
	result := validInspectionFixture()
	result.File = file
	result.Status = InspectionStatusWarning
	result.Repairability = RepairabilityLossless
	result.Container.Status = HealthStatusWarning
	result.Recommendation.Strategy = RepairStrategyRemux

	plan, err := BuildRepairPlan(result)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file.Path, []byte("changed-file"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := PreflightRepair(plan); err == nil || !strings.Contains(err.Error(), "변경") {
		t.Fatalf("expected changed source rejection, got %v", err)
	}
}

func TestRequiredRepairFreeBytesUsesStrategy(t *testing.T) {
	base := RepairPlan{Strategy: RepairStrategyRemux, DurationSeconds: 100}
	remux := requiredRepairFreeBytes(base, 1024*1024*1024)

	reencodePlan := base
	reencodePlan.Strategy = RepairStrategyReencode
	reencode := requiredRepairFreeBytes(reencodePlan, 1024*1024*1024)
	if reencode <= remux {
		t.Fatalf("reencode must reserve more space: remux=%d reencode=%d", remux, reencode)
	}

	truncatePlan := base
	truncatePlan.Strategy = RepairStrategyTruncate
	truncatePlan.EndSeconds = 25
	truncate := requiredRepairFreeBytes(truncatePlan, 1024*1024*1024)
	if truncate >= remux {
		t.Fatalf("truncate estimate must be smaller: truncate=%d remux=%d", truncate, remux)
	}
}

func TestValidateRepairDiskSpaceReportsShortage(t *testing.T) {
	required := int64(2 * 1024 * 1024 * 1024)
	available := uint64(512 * 1024 * 1024)
	if err := validateRepairDiskSpace(required, available); err == nil || !strings.Contains(err.Error(), "부족") {
		t.Fatalf("expected disk shortage error, got %v", err)
	}
	if err := validateRepairDiskSpace(required, uint64(required)); err != nil {
		t.Fatalf("sufficient disk space must pass: %v", err)
	}
}

func TestBuildRepairPlanCapturesInspectionModeAndSourceSignature(t *testing.T) {
	file := repairPlanFixture(t)
	result := validInspectionFixture()
	result.File = file
	result.Mode = InspectionModeDeep
	result.Status = InspectionStatusWarning
	result.Repairability = RepairabilityLossless
	result.Container.Status = HealthStatusWarning
	result.Recommendation.Strategy = RepairStrategyRemux

	plan, err := BuildRepairPlan(result)
	if err != nil {
		t.Fatal(err)
	}
	if plan.InspectionMode != InspectionModeDeep {
		t.Fatalf("deep inspection must remain the repair basis: %#v", plan)
	}
	if plan.SourceSizeBytes != file.SizeBytes || plan.SourceModifiedUnixMilli != file.ModifiedUnixMilli {
		t.Fatalf("source signature was not captured: %#v", plan)
	}
	if plan.RequiredFreeBytes <= 0 {
		t.Fatalf("required disk space was not estimated: %#v", plan)
	}
}

func TestBuildRepairFFmpegArgs(t *testing.T) {
	strategies := []struct {
		strategy RepairStrategy
		end      float64
		want     []string
	}{
		{RepairStrategyRemux, 0, []string{"-c", "copy"}},
		{RepairStrategyTimestampRemux, 0, []string{"+genpts+discardcorrupt", "-avoid_negative_ts", "make_zero"}},
		{RepairStrategyPartial, 0, []string{"ignore_err", "-c", "copy"}},
		{RepairStrategyTruncate, 70, []string{"-t", "70.000"}},
		{RepairStrategyReencode, 0, []string{"libx264", "aac"}},
	}
	for _, test := range strategies {
		t.Run(string(test.strategy), func(t *testing.T) {
			plan := RepairPlan{
				SourcePath:      "sample.mp4",
				OutputPath:      "sample.repaired.mp4",
				Strategy:        test.strategy,
				Executable:      true,
				DurationSeconds: 120,
				EndSeconds:      test.end,
			}
			args, err := buildRepairFFmpegArgs(plan, "sample.repairing.mp4")
			if err != nil {
				t.Fatal(err)
			}
			joined := strings.Join(args, " ")
			for _, expected := range test.want {
				if !strings.Contains(joined, expected) {
					t.Fatalf("missing %q in %s", expected, joined)
				}
			}
		})
	}
}

func TestBuildRepairFFmpegArgsNormalizesUnstableTimestamps(t *testing.T) {
	plan := RepairPlan{
		SourcePath:          "sample.mp4",
		OutputPath:          "sample.repaired.mp4",
		Strategy:            RepairStrategyReencode,
		Executable:          true,
		NormalizeTimestamps: true,
		HasVideo:            true,
		HasAudio:            true,
		DurationSeconds:     120,
	}
	args, err := buildRepairFFmpegArgs(plan, "sample.repairing.mp4")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, expected := range []string{"-fps_mode cfr", "aresample=async=1:first_pts=0"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("missing %q in %s", expected, joined)
		}
	}
}

func TestRepairReencodeArgsByContainer(t *testing.T) {
	webm := strings.Join(repairReencodeArgs(".webm"), " ")
	if !strings.Contains(webm, "libvpx-vp9") || !strings.Contains(webm, "libopus") {
		t.Fatalf("unexpected webm encoders: %s", webm)
	}
	avi := strings.Join(repairReencodeArgs(".avi"), " ")
	if !strings.Contains(avi, "mpeg4") || !strings.Contains(avi, "mp3") {
		t.Fatalf("unexpected avi encoders: %s", avi)
	}
}

func repairPlanFixture(t *testing.T) FileInfo {
	t.Helper()
	directory := t.TempDir()
	path := filepath.Join(directory, "sample.mp4")
	if err := os.WriteFile(path, []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return FileInfo{
		Path:              path,
		Name:              filepath.Base(path),
		SizeBytes:         7,
		ModifiedUnixMilli:  stat.ModTime().UnixNano(),
		Extension:         ".mp4",
		Container:         "MP4",
		ContainerSource:   "probe",
		DurationSeconds:   120,
		MetadataAvailable: true,
	}
}
