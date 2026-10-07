package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MoonKim-isMe/chzzk-video-downloader/internal/downloader"
	"github.com/MoonKim-isMe/chzzk-video-downloader/internal/videorepair"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type repairTestDownloadService struct {
	status downloader.ToolchainStatus
}

func (s repairTestDownloadService) ToolchainStatus(context.Context) downloader.ToolchainStatus {
	return s.status
}

func (repairTestDownloadService) Download(
	context.Context,
	downloader.DownloadRequest,
	downloader.ProgressHandler,
) (downloader.DownloadResult, error) {
	return downloader.DownloadResult{}, nil
}

func TestSelectVideoRepairFileUsesVideoFilterAndMetadataProbe(t *testing.T) {
	selectedPath := filepath.Join(t.TempDir(), "broken.mp4")
	if err := os.WriteFile(selectedPath, []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	previousPath := filepath.Join(filepath.Dir(selectedPath), "previous.mp4")

	app := NewApp()
	app.downloadManager = repairTestDownloadService{
		status: downloader.ToolchainStatus{
			FFprobe: downloader.ToolStatus{Available: true, Path: "ffprobe-test"},
		},
	}

	var options runtime.OpenDialogOptions
	app.filePicker = func(_ context.Context, next runtime.OpenDialogOptions) (string, error) {
		options = next
		return selectedPath, nil
	}

	probeCalled := false
	app.videoFileProbe = func(_ context.Context, ffprobePath, path string) (videorepair.FileInfo, error) {
		probeCalled = true
		if ffprobePath != "ffprobe-test" {
			t.Fatalf("unexpected ffprobe path: %s", ffprobePath)
		}
		if path != selectedPath {
			t.Fatalf("unexpected selected path: %s", path)
		}
		return videorepair.FileInfo{
			Path:              path,
			Name:              filepath.Base(path),
			SizeBytes:         7,
			Extension:         ".mp4",
			Container:         "MP4",
			ContainerSource:   "probe",
			DurationSeconds:   123.5,
			MetadataAvailable: true,
		}, nil
	}

	info, err := app.SelectVideoRepairFile(previousPath)
	if err != nil {
		t.Fatal(err)
	}
	if !probeCalled {
		t.Fatal("metadata probe was not called")
	}
	if info.Path != selectedPath {
		t.Fatalf("unexpected file info: %#v", info)
	}
	if options.Title != "검사할 동영상 선택" {
		t.Fatalf("unexpected dialog title: %s", options.Title)
	}
	if options.DefaultDirectory != filepath.Dir(previousPath) {
		t.Fatalf("unexpected default directory: %s", options.DefaultDirectory)
	}
	if len(options.Filters) != 1 || !strings.Contains(options.Filters[0].Pattern, "*.mp4") {
		t.Fatalf("video filter was not applied: %#v", options.Filters)
	}
}

func TestSelectVideoRepairFileCancelDoesNotProbe(t *testing.T) {
	app := NewApp()
	app.downloadManager = repairTestDownloadService{}
	app.filePicker = func(context.Context, runtime.OpenDialogOptions) (string, error) {
		return "", nil
	}

	probeCalled := false
	app.videoFileProbe = func(context.Context, string, string) (videorepair.FileInfo, error) {
		probeCalled = true
		return videorepair.FileInfo{}, nil
	}

	info, err := app.SelectVideoRepairFile("")
	if err != nil {
		t.Fatal(err)
	}
	if probeCalled {
		t.Fatal("metadata probe must not run after dialog cancellation")
	}
	if info.Path != "" {
		t.Fatalf("cancelled selection must be empty: %#v", info)
	}
}

func TestStartQuickVideoInspectionUsesBundledToolchainAndProgress(t *testing.T) {
	app := NewApp()
	app.downloadManager = repairTestDownloadService{
		status: downloader.ToolchainStatus{
			FFprobe: downloader.ToolStatus{Available: true, Path: "ffprobe-test"},
			FFmpeg:  downloader.ToolStatus{Available: true, Path: "ffmpeg-test"},
		},
	}

	var emitted []videorepair.InspectionProgress
	app.inspectionProgressEmitter = func(progress videorepair.InspectionProgress) {
		emitted = append(emitted, progress)
	}
	app.quickVideoInspector = func(
		ctx context.Context,
		ffprobePath string,
		ffmpegPath string,
		path string,
		handler videorepair.ProgressHandler,
	) (videorepair.InspectionResult, error) {
		if ffprobePath != "ffprobe-test" || ffmpegPath != "ffmpeg-test" {
			t.Fatalf("unexpected tool paths: %s / %s", ffprobePath, ffmpegPath)
		}
		if path != "C:/video/sample.mp4" {
			t.Fatalf("unexpected inspection path: %s", path)
		}
		handler(videorepair.InspectionProgress{
			Mode:    videorepair.InspectionModeQuick,
			Percent: 50,
			Stage:   "대표 구간 확인 중",
		})
		return videorepair.InspectionResult{
			Mode:          videorepair.InspectionModeQuick,
			Status:        videorepair.InspectionStatusNormal,
			Repairability: videorepair.RepairabilityNotNeeded,
		}, nil
	}

	result, err := app.StartQuickVideoInspection("C:/video/sample.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != videorepair.InspectionStatusNormal {
		t.Fatalf("unexpected inspection result: %#v", result)
	}
	if len(emitted) != 1 || emitted[0].Percent != 50 {
		t.Fatalf("unexpected progress events: %#v", emitted)
	}
}

func TestCancelVideoInspectionCancelsRunningContext(t *testing.T) {
	app := NewApp()
	app.downloadManager = repairTestDownloadService{
		status: downloader.ToolchainStatus{
			FFprobe: downloader.ToolStatus{Available: true, Path: "ffprobe-test"},
			FFmpeg:  downloader.ToolStatus{Available: true, Path: "ffmpeg-test"},
		},
	}

	started := make(chan struct{})
	app.quickVideoInspector = func(
		ctx context.Context,
		_ string,
		_ string,
		_ string,
		_ videorepair.ProgressHandler,
	) (videorepair.InspectionResult, error) {
		close(started)
		<-ctx.Done()
		return videorepair.InspectionResult{}, ctx.Err()
	}

	done := make(chan error, 1)
	go func() {
		_, err := app.StartQuickVideoInspection("C:/video/sample.mp4")
		done <- err
	}()

	<-started
	if !app.CancelVideoInspection() {
		t.Fatal("expected active inspection to be cancelled")
	}
	if err := <-done; err == nil || !strings.Contains(err.Error(), "취소") {
		t.Fatalf("unexpected cancellation result: %v", err)
	}
	if app.CancelVideoInspection() {
		t.Fatal("inspection must no longer be active")
	}
}

func TestStartQuickVideoInspectionRequiresToolchain(t *testing.T) {
	app := NewApp()
	app.downloadManager = repairTestDownloadService{}

	_, err := app.StartQuickVideoInspection("C:/video/sample.mp4")
	if err == nil {
		t.Fatal("expected missing toolchain error")
	}
	if errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected cancellation error: %v", err)
	}
}

func TestStartDeepVideoInspectionUsesBundledToolchainAndProgress(t *testing.T) {
	app := NewApp()
	app.downloadManager = repairTestDownloadService{
		status: downloader.ToolchainStatus{
			FFprobe: downloader.ToolStatus{Available: true, Path: "ffprobe-test"},
			FFmpeg:  downloader.ToolStatus{Available: true, Path: "ffmpeg-test"},
		},
	}

	var emitted []videorepair.InspectionProgress
	app.inspectionProgressEmitter = func(progress videorepair.InspectionProgress) {
		emitted = append(emitted, progress)
	}
	app.deepVideoInspector = func(
		ctx context.Context,
		ffprobePath string,
		ffmpegPath string,
		path string,
		handler videorepair.ProgressHandler,
	) (videorepair.InspectionResult, error) {
		if ffprobePath != "ffprobe-test" || ffmpegPath != "ffmpeg-test" {
			t.Fatalf("unexpected tool paths: %s / %s", ffprobePath, ffmpegPath)
		}
		if path != "C:/video/deep.mp4" {
			t.Fatalf("unexpected inspection path: %s", path)
		}
		handler(videorepair.InspectionProgress{
			Mode:             videorepair.InspectionModeDeep,
			Path:             path,
			Percent:          50,
			Stage:            "전체 스트림 디코딩 중",
			ProcessedSeconds: 60,
			Speed:            2.5,
			ElapsedSeconds:   24,
		})
		return videorepair.InspectionResult{
			Mode:          videorepair.InspectionModeDeep,
			Status:        videorepair.InspectionStatusNormal,
			Repairability: videorepair.RepairabilityNotNeeded,
		}, nil
	}

	result, err := app.StartDeepVideoInspection("C:/video/deep.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != videorepair.InspectionModeDeep {
		t.Fatalf("unexpected result: %#v", result)
	}
	if len(emitted) != 1 || emitted[0].ProcessedSeconds != 60 || emitted[0].Speed != 2.5 {
		t.Fatalf("unexpected deep progress: %#v", emitted)
	}
}

func TestStartVideoRepairUsesBundledToolchainAndProgress(t *testing.T) {
	app := NewApp()
	app.downloadManager = repairTestDownloadService{
		status: downloader.ToolchainStatus{
			FFprobe: downloader.ToolStatus{Available: true, Path: "ffprobe-test"},
			FFmpeg:  downloader.ToolStatus{Available: true, Path: "ffmpeg-test"},
		},
	}

	var emitted []videorepair.RepairProgress
	app.repairProgressEmitter = func(progress videorepair.RepairProgress) {
		emitted = append(emitted, progress)
	}
	app.videoRepairer = func(
		ctx context.Context,
		ffprobePath string,
		ffmpegPath string,
		plan videorepair.RepairPlan,
		handler videorepair.RepairProgressHandler,
	) (videorepair.RepairResult, error) {
		if ffprobePath != "ffprobe-test" || ffmpegPath != "ffmpeg-test" {
			t.Fatalf("unexpected repair tool paths: %s / %s", ffprobePath, ffmpegPath)
		}
		handler(videorepair.RepairProgress{
			SourcePath: plan.SourcePath,
			OutputPath: plan.OutputPath,
			Strategy:   plan.Strategy,
			Percent:    50,
			Stage:      "복구 파일 생성 중",
		})
		return videorepair.RepairResult{Plan: plan}, nil
	}

	plan := videorepair.RepairPlan{
		SourcePath:      "C:/video/sample.mp4",
		OutputPath:      "C:/video/sample.repaired.mp4",
		Strategy:        videorepair.RepairStrategyRemux,
		Executable:      true,
		DurationSeconds: 120,
	}
	result, err := app.StartVideoRepair(plan)
	if err != nil {
		t.Fatal(err)
	}
	if result.Plan.OutputPath != plan.OutputPath {
		t.Fatalf("unexpected repair result: %#v", result)
	}
	if len(emitted) != 1 || emitted[0].Percent != 50 {
		t.Fatalf("unexpected repair progress: %#v", emitted)
	}
}

func TestCancelVideoRepairCancelsRunningContext(t *testing.T) {
	app := NewApp()
	app.downloadManager = repairTestDownloadService{
		status: downloader.ToolchainStatus{
			FFprobe: downloader.ToolStatus{Available: true, Path: "ffprobe-test"},
			FFmpeg:  downloader.ToolStatus{Available: true, Path: "ffmpeg-test"},
		},
	}

	started := make(chan struct{})
	app.videoRepairer = func(
		ctx context.Context,
		_ string,
		_ string,
		_ videorepair.RepairPlan,
		_ videorepair.RepairProgressHandler,
	) (videorepair.RepairResult, error) {
		close(started)
		<-ctx.Done()
		return videorepair.RepairResult{}, ctx.Err()
	}

	done := make(chan error, 1)
	go func() {
		_, err := app.StartVideoRepair(videorepair.RepairPlan{
			SourcePath:      "C:/video/sample.mp4",
			OutputPath:      "C:/video/sample.repaired.mp4",
			Strategy:        videorepair.RepairStrategyRemux,
			Executable:      true,
			DurationSeconds: 120,
		})
		done <- err
	}()

	<-started
	if !app.CancelVideoRepair() {
		t.Fatal("expected active repair to be cancelled")
	}
	if err := <-done; err == nil || !strings.Contains(err.Error(), "취소") {
		t.Fatalf("unexpected repair cancellation result: %v", err)
	}
}

func TestRevealVideoRepairFileUsesFileRevealer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "result.mp4")
	if err := os.WriteFile(path, []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}

	app := NewApp()
	revealed := ""
	app.fileRevealer = func(next string) error {
		revealed = next
		return nil
	}

	if err := app.RevealVideoRepairFile(path); err != nil {
		t.Fatal(err)
	}
	if revealed != path {
		t.Fatalf("unexpected revealed path: %s", revealed)
	}
}

func TestShutdownCancelsRunningVideoInspection(t *testing.T) {
	app := NewApp()
	app.downloadManager = repairTestDownloadService{
		status: downloader.ToolchainStatus{
			FFprobe: downloader.ToolStatus{Available: true, Path: "ffprobe-test"},
			FFmpeg:  downloader.ToolStatus{Available: true, Path: "ffmpeg-test"},
		},
	}

	started := make(chan struct{})
	app.quickVideoInspector = func(
		ctx context.Context,
		_ string,
		_ string,
		_ string,
		_ videorepair.ProgressHandler,
	) (videorepair.InspectionResult, error) {
		close(started)
		<-ctx.Done()
		return videorepair.InspectionResult{}, ctx.Err()
	}

	done := make(chan error, 1)
	go func() {
		_, err := app.StartQuickVideoInspection("C:/video/shutdown.mp4")
		done <- err
	}()

	<-started
	app.shutdown(context.Background())
	if err := <-done; err == nil || !strings.Contains(err.Error(), "취소") {
		t.Fatalf("shutdown must cancel inspection: %v", err)
	}
}

func TestShutdownCancelsRunningVideoRepair(t *testing.T) {
	app := NewApp()
	app.downloadManager = repairTestDownloadService{
		status: downloader.ToolchainStatus{
			FFprobe: downloader.ToolStatus{Available: true, Path: "ffprobe-test"},
			FFmpeg:  downloader.ToolStatus{Available: true, Path: "ffmpeg-test"},
		},
	}

	started := make(chan struct{})
	app.videoRepairer = func(
		ctx context.Context,
		_ string,
		_ string,
		_ videorepair.RepairPlan,
		_ videorepair.RepairProgressHandler,
	) (videorepair.RepairResult, error) {
		close(started)
		<-ctx.Done()
		return videorepair.RepairResult{}, ctx.Err()
	}

	done := make(chan error, 1)
	go func() {
		_, err := app.StartVideoRepair(videorepair.RepairPlan{
			SourcePath: "C:/video/shutdown.mp4",
			OutputPath: "C:/video/shutdown.repaired.mp4",
			Strategy: videorepair.RepairStrategyRemux,
			Executable: true,
		})
		done <- err
	}()

	<-started
	app.shutdown(context.Background())
	if err := <-done; err == nil || !strings.Contains(err.Error(), "취소") {
		t.Fatalf("shutdown must cancel repair: %v", err)
	}
}
