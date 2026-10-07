package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/MoonKim-isMe/chzzk-video-downloader/internal/downloader"
	"github.com/MoonKim-isMe/chzzk-video-downloader/internal/videorepair"
)

func TestVideoRepairScenarioQuickThenRepair(t *testing.T) {
	app, file := newRepairScenarioApp(t)
	app.quickVideoInspector = func(
		context.Context, string, string, string, videorepair.ProgressHandler,
	) (videorepair.InspectionResult, error) {
		return repairScenarioResult(file, videorepair.InspectionModeQuick, false), nil
	}
	app.videoRepairer = repairScenarioStub

	quick, err := app.StartQuickVideoInspection(file.Path)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := app.CreateVideoRepairPlan(quick)
	if err != nil {
		t.Fatal(err)
	}
	if plan.InspectionMode != videorepair.InspectionModeQuick {
		t.Fatalf("quick result must be the repair basis: %#v", plan)
	}
	if _, err := app.StartVideoRepair(plan); err != nil {
		t.Fatal(err)
	}
}

func TestVideoRepairScenarioQuickThenDeepThenRepairUsesDeepResult(t *testing.T) {
	app, file := newRepairScenarioApp(t)
	app.quickVideoInspector = func(
		context.Context, string, string, string, videorepair.ProgressHandler,
	) (videorepair.InspectionResult, error) {
		return repairScenarioResult(file, videorepair.InspectionModeQuick, true), nil
	}
	app.deepVideoInspector = func(
		context.Context, string, string, string, videorepair.ProgressHandler,
	) (videorepair.InspectionResult, error) {
		result := repairScenarioResult(file, videorepair.InspectionModeDeep, false)
		result.Container.Status = videorepair.HealthStatusNormal
		result.Timestamps.Status = videorepair.HealthStatusWarning
		result.Timestamps.DTSErrorCount = 2
		result.Recommendation.Strategy = videorepair.RepairStrategyTimestampRemux
		return result, nil
	}
	app.videoRepairer = repairScenarioStub

	quick, err := app.StartQuickVideoInspection(file.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !quick.DeepInspectionRecommended {
		t.Fatal("quick inspection must recommend deep inspection in this scenario")
	}

	deep, err := app.StartDeepVideoInspection(file.Path)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := app.CreateVideoRepairPlan(deep)
	if err != nil {
		t.Fatal(err)
	}
	if plan.InspectionMode != videorepair.InspectionModeDeep {
		t.Fatalf("deep result must take priority for repair: %#v", plan)
	}
	if plan.Strategy != videorepair.RepairStrategyTimestampRemux {
		t.Fatalf("unexpected deep repair strategy: %#v", plan)
	}
	if _, err := app.StartVideoRepair(plan); err != nil {
		t.Fatal(err)
	}
}

func TestVideoRepairScenarioDeepThenRepair(t *testing.T) {
	app, file := newRepairScenarioApp(t)
	app.deepVideoInspector = func(
		context.Context, string, string, string, videorepair.ProgressHandler,
	) (videorepair.InspectionResult, error) {
		return repairScenarioResult(file, videorepair.InspectionModeDeep, false), nil
	}
	app.videoRepairer = repairScenarioStub

	deep, err := app.StartDeepVideoInspection(file.Path)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := app.CreateVideoRepairPlan(deep)
	if err != nil {
		t.Fatal(err)
	}
	if plan.InspectionMode != videorepair.InspectionModeDeep {
		t.Fatalf("deep inspection must be the repair basis: %#v", plan)
	}
	if _, err := app.StartVideoRepair(plan); err != nil {
		t.Fatal(err)
	}
}

func newRepairScenarioApp(t *testing.T) (*App, videorepair.FileInfo) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "scenario.mp4")
	if err := os.WriteFile(path, []byte("scenario-fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	file := videorepair.FileInfo{
		Path:              path,
		Name:              filepath.Base(path),
		SizeBytes:         stat.Size(),
		ModifiedUnixMilli:  stat.ModTime().UnixMilli(),
		Extension:         ".mp4",
		Container:         "MP4",
		ContainerSource:   "probe",
		DurationSeconds:   120,
		MetadataAvailable: true,
	}
	app := NewApp()
	app.downloadManager = repairTestDownloadService{
		status: downloader.ToolchainStatus{
			FFprobe: downloader.ToolStatus{Available: true, Path: "ffprobe-test"},
			FFmpeg:  downloader.ToolStatus{Available: true, Path: "ffmpeg-test"},
		},
	}
	return app, file
}

func repairScenarioResult(
	file videorepair.FileInfo,
	mode videorepair.InspectionMode,
	recommendDeep bool,
) videorepair.InspectionResult {
	return videorepair.InspectionResult{
		Mode:          mode,
		Status:        videorepair.InspectionStatusWarning,
		Repairability: videorepair.RepairabilityLossless,
		Summary:       "복구 가능한 경고가 확인되었습니다.",
		File:          file,
		Container: videorepair.ContainerInspection{
			Status:                 videorepair.HealthStatusWarning,
			Parseable:              true,
			StreamMetadataReadable: true,
			IndexStatus:            videorepair.HealthStatusNormal,
			MoovAtomStatus:         videorepair.MoovAtomPresent,
		},
		Video: videorepair.VideoStreamInspection{
			Status:  videorepair.HealthStatusNormal,
			Present: true,
			Codec:   "h264",
		},
		Audio: videorepair.AudioStreamInspection{
			Status:  videorepair.HealthStatusNormal,
			Present: true,
			Codec:   "aac",
		},
		Timestamps: videorepair.TimestampInspection{Status: videorepair.HealthStatusNormal},
		AVSync:     videorepair.AVSyncInspection{Status: videorepair.HealthStatusNormal},
		DamageRanges: []videorepair.DamageRange{},
		Recommendation: videorepair.RepairRecommendation{
			Strategy: videorepair.RepairStrategyRemux,
			Summary:  "무손실 Remux를 권장합니다.",
		},
		DeepInspectionRecommended: recommendDeep,
		DeepInspectionReason: func() string {
			if recommendDeep {
				return "빠른 검사만으로 전체 구간을 확정할 수 없어 정밀 검사를 권장합니다."
			}
			return ""
		}(),
	}
}

func repairScenarioStub(
	_ context.Context,
	_ string,
	_ string,
	plan videorepair.RepairPlan,
	_ videorepair.RepairProgressHandler,
) (videorepair.RepairResult, error) {
	return videorepair.RepairResult{Plan: plan}, nil
}
