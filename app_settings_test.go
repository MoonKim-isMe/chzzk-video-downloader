package main

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/MoonKim-isMe/chzzk-video-downloader/internal/downloader"
	appsettings "github.com/MoonKim-isMe/chzzk-video-downloader/internal/settings"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func TestGetSettingsReturnsDefaults(t *testing.T) {
	app := NewApp()
	store, err := appsettings.NewStore(appsettings.Defaults(filepath.Join(t.TempDir(), "downloads")))
	if err != nil {
		t.Fatal(err)
	}
	app.settingsStore = store

	value, err := app.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	if value.Resolution != appsettings.ResolutionBest ||
		value.OutputFormat != appsettings.OutputFormatMP4 ||
		value.DownloadAcceleration != appsettings.DownloadAccelerationStandard ||
		value.DownloadRateLimitMBps != 0 ||
		value.MaxConcurrentDownloads != 3 ||
		value.Theme != appsettings.ThemeDark {
		t.Fatalf("unexpected settings: %#v", value)
	}
}

func TestUpdateSettingsStoresNormalizedValue(t *testing.T) {
	app := NewApp()
	store, err := appsettings.NewStore(appsettings.Defaults(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	app.settingsStore = store

	next, err := app.UpdateSettings(appsettings.AppSettings{
		DownloadDir:            filepath.Join(t.TempDir(), "video"),
		Resolution:             appsettings.Resolution(" 1440P "),
		OutputFormat:           appsettings.OutputFormat(" WEBM "),
		DownloadAcceleration:   appsettings.DownloadAcceleration(" FAST "),
		DownloadRateLimitMBps:   12.5,
		MaxConcurrentDownloads: 3,
		Theme:                  appsettings.ThemeLight,
	})
	if err != nil {
		t.Fatal(err)
	}

	if next.Resolution != appsettings.Resolution1440p ||
		next.OutputFormat != appsettings.OutputFormatWebM ||
		next.DownloadAcceleration != appsettings.DownloadAccelerationFast ||
		next.DownloadRateLimitMBps != 12.5 ||
		next.MaxConcurrentDownloads != 3 ||
		next.Theme != appsettings.ThemeLight {
		t.Fatalf("unexpected normalized settings: %#v", next)
	}

	current, err := app.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	if current != next {
		t.Fatalf("settings mismatch: current=%#v next=%#v", current, next)
	}
}

func TestUpdateSettingsAppliesQueueConcurrency(t *testing.T) {
	app := NewApp()
	store, err := appsettings.NewStore(appsettings.Defaults(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	app.settingsStore = store
	app.downloadManager = &fakeDownloadService{
		status: readyDownloadStatus(),
		download: func(
			context.Context,
			downloader.DownloadRequest,
			downloader.ProgressHandler,
		) (downloader.DownloadResult, error) {
			return downloader.DownloadResult{}, nil
		},
	}

	queue := app.ensureDownloadQueue()
	if queue.MaxConcurrent() != 3 {
		t.Fatalf("unexpected initial concurrency: %d", queue.MaxConcurrent())
	}

	updated, err := app.UpdateSettings(appsettings.AppSettings{
		DownloadDir:            t.TempDir(),
		Resolution:             appsettings.Resolution1080p,
		OutputFormat:           appsettings.OutputFormatMKV,
		MaxConcurrentDownloads: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.MaxConcurrentDownloads != 4 || queue.MaxConcurrent() != 4 {
		t.Fatalf("concurrency was not applied: settings=%d queue=%d", updated.MaxConcurrentDownloads, queue.MaxConcurrent())
	}
}

func TestEnsureDownloadQueueUsesCurrentSettingsConcurrency(t *testing.T) {
	app := NewApp()
	store, err := appsettings.NewStore(appsettings.AppSettings{
		DownloadDir:            t.TempDir(),
		Resolution:             appsettings.ResolutionBest,
		OutputFormat:           appsettings.OutputFormatMP4,
		MaxConcurrentDownloads: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	app.settingsStore = store
	app.downloadManager = &fakeDownloadService{
		status: readyDownloadStatus(),
		download: func(
			context.Context,
			downloader.DownloadRequest,
			downloader.ProgressHandler,
		) (downloader.DownloadResult, error) {
			return downloader.DownloadResult{}, nil
		},
	}

	if actual := app.ensureDownloadQueue().MaxConcurrent(); actual != 5 {
		t.Fatalf("unexpected queue concurrency: %d", actual)
	}
}

func TestUpdateSettingsRejectsInvalidValueWithoutChangingStore(t *testing.T) {
	app := NewApp()
	store, err := appsettings.NewStore(appsettings.Defaults(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	app.settingsStore = store
	before := store.Get()

	_, err = app.UpdateSettings(appsettings.AppSettings{
		DownloadDir:            t.TempDir(),
		Resolution:             "480p",
		OutputFormat:           appsettings.OutputFormatMP4,
		MaxConcurrentDownloads: 1,
	})
	if err == nil {
		t.Fatal("expected validation error")
	}

	after, err := app.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("invalid update changed settings: before=%#v after=%#v", before, after)
	}
}


func TestSelectDownloadDirectoryUsesCurrentDirectory(t *testing.T) {
	app := NewApp()
	expected := filepath.Join(t.TempDir(), "selected")
	var gotDefault string

	app.directoryPicker = func(ctx context.Context, options runtime.OpenDialogOptions) (string, error) {
		gotDefault = options.DefaultDirectory
		if options.Title != "다운로드 폴더 선택" {
			t.Fatalf("unexpected title: %s", options.Title)
		}
		return expected, nil
	}

	current := filepath.Join(t.TempDir(), "current")
	selected, err := app.SelectDownloadDirectory(current)
	if err != nil {
		t.Fatal(err)
	}
	if gotDefault != current {
		t.Fatalf("unexpected default directory: got %s want %s", gotDefault, current)
	}
	if selected != expected {
		t.Fatalf("unexpected selected directory: got %s want %s", selected, expected)
	}
}

func TestSelectDownloadDirectoryFallsBackToSettingsAndAllowsCancel(t *testing.T) {
	app := NewApp()
	defaultDir := filepath.Join(t.TempDir(), "downloads")
	store, err := appsettings.NewStore(appsettings.Defaults(defaultDir))
	if err != nil {
		t.Fatal(err)
	}
	app.settingsStore = store

	app.directoryPicker = func(ctx context.Context, options runtime.OpenDialogOptions) (string, error) {
		if options.DefaultDirectory != defaultDir {
			t.Fatalf("unexpected settings fallback: got %s want %s", options.DefaultDirectory, defaultDir)
		}
		return "", nil
	}

	selected, err := app.SelectDownloadDirectory("")
	if err != nil {
		t.Fatal(err)
	}
	if selected != "" {
		t.Fatalf("cancel should return empty selection: %q", selected)
	}
}

func TestSelectDownloadDirectoryWrapsDialogError(t *testing.T) {
	app := NewApp()
	app.directoryPicker = func(context.Context, runtime.OpenDialogOptions) (string, error) {
		return "", errors.New("dialog failed")
	}

	if _, err := app.SelectDownloadDirectory(t.TempDir()); err == nil {
		t.Fatal("expected dialog error")
	}
}
