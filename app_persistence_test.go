package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/MoonKim-isMe/chzzk-video-downloader/internal/chzzk"
	"github.com/MoonKim-isMe/chzzk-video-downloader/internal/downloader"
	"github.com/MoonKim-isMe/chzzk-video-downloader/internal/persistence"
	appsettings "github.com/MoonKim-isMe/chzzk-video-downloader/internal/settings"
)

const appPersistenceTestChannelID = "6e06f5e1907f17eff543abd06cb62891"

func TestInitializePersistenceRestoresChannelsSettingsAndHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.sqlite3")
	seed, err := persistence.Open(path)
	if err != nil {
		t.Fatal(err)
	}

	channel := chzzk.Channel{
		ChannelID:   appPersistenceTestChannelID,
		ChannelName: "복원 채널",
		ChannelURL:  chzzk.ChannelURL(appPersistenceTestChannelID),
	}
	if err := seed.UpsertChannel(channel); err != nil {
		t.Fatal(err)
	}

	settingsValue := appsettings.AppSettings{
		DownloadDir:            filepath.Join(t.TempDir(), "persisted-downloads"),
		Resolution:             appsettings.Resolution1440p,
		OutputFormat:           appsettings.OutputFormatMKV,
		DownloadAcceleration:   appsettings.DownloadAccelerationUltra,
		DownloadRateLimitMBps:   12.5,
		MaxConcurrentDownloads: 3,
		Theme:                  appsettings.ThemeLight,
	}
	record, err := appsettings.NewStorageRecord(settingsValue)
	if err != nil {
		t.Fatal(err)
	}
	if err := seed.SaveSettings(record); err != nil {
		t.Fatal(err)
	}

	running := downloader.DownloadTask{
		TaskID:      "persisted-running",
		VideoNo:     12345,
		VideoTitle:  "중단 VOD",
		ChannelName: "복원 채널",
		URL:         "https://chzzk.naver.com/video/12345",
		OutputDir:   settingsValue.DownloadDir,
		Status:      downloader.TaskStatusRunning,
		QueuedAt:    "2026-10-01T00:00:00Z",
		StartedAt:   "2026-10-01T00:00:01Z",
	}
	if err := seed.UpsertDownloadTask(running); err != nil {
		t.Fatal(err)
	}
	if err := seed.Close(); err != nil {
		t.Fatal(err)
	}

	app := NewApp()
	app.ctx = context.Background()
	app.databasePath = func() (string, error) { return path, nil }
	app.eventEmitter = func(downloader.DownloadTask) {}
	if err := app.initializePersistence(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if app.database != nil {
			_ = app.database.Close()
		}
	})

	channels := app.GetSavedChannels()
	if len(channels) != 1 || channels[0].ChannelID != appPersistenceTestChannelID {
		t.Fatalf("unexpected restored channels: %#v", channels)
	}

	current, err := app.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	if current != appsettings.Normalize(settingsValue) {
		t.Fatalf("unexpected restored settings: %#v", current)
	}

	tasks := app.GetDownloadTasks()
	if len(tasks) != 0 {
		t.Fatalf("recovered cancelled history must be hidden: %#v", tasks)
	}
	if app.ensureDownloadQueue().MaxConcurrent() != settingsValue.MaxConcurrentDownloads {
		t.Fatalf("restored concurrency was not applied: %d", app.ensureDownloadQueue().MaxConcurrent())
	}
}

func TestSavedChannelChangesArePersisted(t *testing.T) {
	database, err := persistence.Open(filepath.Join(t.TempDir(), "app.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	app := NewApp()
	app.database = database

	channel := chzzk.Channel{ChannelID: appPersistenceTestChannelID, ChannelName: "저장 채널"}
	if _, err := app.SaveChannel(channel); err != nil {
		t.Fatal(err)
	}

	stored, err := database.ListChannels()
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].ChannelID != appPersistenceTestChannelID {
		t.Fatalf("channel was not persisted: %#v", stored)
	}

	if _, err := app.RemoveSavedChannel(appPersistenceTestChannelID); err != nil {
		t.Fatal(err)
	}
	stored, err = database.ListChannels()
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 0 {
		t.Fatalf("channel was not deleted: %#v", stored)
	}
}

func TestUpdateSettingsPersistsStorageRecord(t *testing.T) {
	database, err := persistence.Open(filepath.Join(t.TempDir(), "app.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	store, err := appsettings.NewStore(appsettings.Defaults(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	app := NewApp()
	app.database = database
	app.settingsStore = store

	next := appsettings.AppSettings{
		DownloadDir:            filepath.Join(t.TempDir(), "new-downloads"),
		Resolution:             appsettings.Resolution720p,
		OutputFormat:           appsettings.OutputFormatWebM,
		DownloadAcceleration:   appsettings.DownloadAccelerationStable,
		DownloadRateLimitMBps:   8.75,
		MaxConcurrentDownloads: 2,
		Theme:                  appsettings.ThemeLight,
	}
	if _, err := app.UpdateSettings(next); err != nil {
		t.Fatal(err)
	}

	record, found, err := database.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected persisted settings")
	}
	restored, err := record.AppSettings()
	if err != nil {
		t.Fatal(err)
	}
	if restored != appsettings.Normalize(next) {
		t.Fatalf("unexpected persisted settings: %#v", restored)
	}
}

func TestDownloadStateEventPersistsHistory(t *testing.T) {
	database, err := persistence.Open(filepath.Join(t.TempDir(), "app.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	app := NewApp()
	app.ctx = context.Background()
	app.database = database
	app.eventEmitter = func(downloader.DownloadTask) {}

	task := downloader.DownloadTask{
		TaskID:      "history-task",
		VideoNo:     54321,
		VideoTitle:  "이력 VOD",
		ChannelName: "채널",
		URL:         "https://chzzk.naver.com/video/54321",
		OutputDir:   t.TempDir(),
		Status:      downloader.TaskStatusCompleted,
		Progress: downloader.DownloadProgress{
			Status:          "completed",
			Percent:         100,
			DownloadedBytes: 1024,
			TotalBytes:      1024,
		},
		FinalPath:  filepath.Join(t.TempDir(), "result.mp4"),
		QueuedAt:   "2026-10-01T00:00:00Z",
		StartedAt:  "2026-10-01T00:00:01Z",
		FinishedAt: "2026-10-01T00:00:10Z",
	}
	app.emitDownloadState(task)

	tasks, err := database.ListDownloadTasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].TaskID != task.TaskID || tasks[0].FinalPath != task.FinalPath {
		t.Fatalf("download history was not persisted: %#v", tasks)
	}
}


func TestCancelledDownloadStateDeletesPersistedHistory(t *testing.T) {
	database, err := persistence.Open(filepath.Join(t.TempDir(), "app.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	task := downloader.DownloadTask{
		TaskID:      "cancelled-history",
		VideoNo:     70001,
		VideoTitle:  "취소 VOD",
		ChannelName: "채널",
		URL:         "https://chzzk.naver.com/video/70001",
		OutputDir:   t.TempDir(),
		Status:      downloader.TaskStatusRunning,
		QueuedAt:    "2026-10-01T00:00:00Z",
	}
	if err := database.UpsertDownloadTask(task); err != nil {
		t.Fatal(err)
	}

	app := NewApp()
	app.ctx = context.Background()
	app.database = database
	app.eventEmitter = func(downloader.DownloadTask) {}

	task.Status = downloader.TaskStatusCancelled
	task.FinishedAt = "2026-10-01T00:00:05Z"
	app.emitDownloadState(task)

	tasks, err := database.ListDownloadTasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 0 {
		t.Fatalf("cancelled history should be deleted: %#v", tasks)
	}
}

func TestDeleteDownloadTaskRemovesPersistedCompletedHistory(t *testing.T) {
	database, err := persistence.Open(filepath.Join(t.TempDir(), "app.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	task := downloader.DownloadTask{
		TaskID:      "completed-delete",
		VideoNo:     70002,
		VideoTitle:  "완료 VOD",
		ChannelName: "채널",
		URL:         "https://chzzk.naver.com/video/70002",
		OutputDir:   t.TempDir(),
		Status:      downloader.TaskStatusCompleted,
		FinalPath:   filepath.Join(t.TempDir(), "done.mp4"),
		QueuedAt:    "2026-10-01T00:00:00Z",
		FinishedAt:  "2026-10-01T00:01:00Z",
	}
	if err := database.UpsertDownloadTask(task); err != nil {
		t.Fatal(err)
	}

	app := NewApp()
	app.ctx = context.Background()
	app.database = database
	app.eventEmitter = func(downloader.DownloadTask) {}

	if err := app.DeleteDownloadTask(task.TaskID); err != nil {
		t.Fatal(err)
	}
	tasks, err := database.ListDownloadTasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 0 {
		t.Fatalf("completed history was not deleted: %#v", tasks)
	}
}

func TestOpenDownloadFolderUsesFinalPathDirectory(t *testing.T) {
	database, err := persistence.Open(filepath.Join(t.TempDir(), "app.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	directory := t.TempDir()
	task := downloader.DownloadTask{
		TaskID:      "completed-folder",
		VideoNo:     70003,
		VideoTitle:  "완료 VOD",
		ChannelName: "채널",
		URL:         "https://chzzk.naver.com/video/70003",
		OutputDir:   filepath.Join(t.TempDir(), "fallback"),
		Status:      downloader.TaskStatusCompleted,
		FinalPath:   filepath.Join(directory, "done.mp4"),
		QueuedAt:    "2026-10-01T00:00:00Z",
		FinishedAt:  "2026-10-01T00:01:00Z",
	}
	if err := database.UpsertDownloadTask(task); err != nil {
		t.Fatal(err)
	}

	app := NewApp()
	app.ctx = context.Background()
	app.database = database
	var opened string
	app.folderOpener = func(path string) error {
		opened = path
		return nil
	}

	if err := app.OpenDownloadFolder(task.TaskID); err != nil {
		t.Fatal(err)
	}
	if opened != directory {
		t.Fatalf("unexpected opened directory: got %q want %q", opened, directory)
	}
}
