package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/MoonKim-isMe/chzzk-video-downloader/internal/downloader"
	appsettings "github.com/MoonKim-isMe/chzzk-video-downloader/internal/settings"
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
		value.MaxConcurrentDownloads != 1 {
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
		MaxConcurrentDownloads: 3,
	})
	if err != nil {
		t.Fatal(err)
	}

	if next.Resolution != appsettings.Resolution1440p ||
		next.OutputFormat != appsettings.OutputFormatWebM ||
		next.MaxConcurrentDownloads != 3 {
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
	if queue.MaxConcurrent() != 1 {
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
