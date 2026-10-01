package persistence

import (
	"path/filepath"
	"testing"

	"github.com/MoonKim-isMe/chzzk-video-downloader/internal/chzzk"
	"github.com/MoonKim-isMe/chzzk-video-downloader/internal/downloader"
	appsettings "github.com/MoonKim-isMe/chzzk-video-downloader/internal/settings"
)

const persistenceTestChannelID = "6e06f5e1907f17eff543abd06cb62891"

func openTestDatabase(t *testing.T) *Database {
	t.Helper()
	database, err := Open(filepath.Join(t.TempDir(), "test.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = database.Close()
	})
	return database
}

func TestOpenAppliesMigrationsIdempotently(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.sqlite3")
	database, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	version, err := database.MigrationVersion()
	if err != nil {
		t.Fatal(err)
	}
	if version != 2 {
		t.Fatalf("unexpected migration version: %d", version)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	version, err = reopened.MigrationVersion()
	if err != nil {
		t.Fatal(err)
	}
	if version != 2 {
		t.Fatalf("unexpected reopened migration version: %d", version)
	}
}

func TestChannelPersistenceRoundTrip(t *testing.T) {
	database := openTestDatabase(t)
	channel := chzzk.Channel{
		ChannelID:          persistenceTestChannelID,
		ChannelName:        "테스트 채널",
		ChannelImageURL:    "https://example.com/channel.png",
		ChannelDescription: "설명",
		FollowerCount:      1234,
		VerifiedMark:       true,
		OpenLive:           true,
		ChannelURL:         chzzk.ChannelURL(persistenceTestChannelID),
	}
	if err := database.UpsertChannel(channel); err != nil {
		t.Fatal(err)
	}

	channels, err := database.ListChannels()
	if err != nil {
		t.Fatal(err)
	}
	if len(channels) != 1 || channels[0] != channel {
		t.Fatalf("unexpected channels: %#v", channels)
	}

	if err := database.DeleteChannel(channel.ChannelID); err != nil {
		t.Fatal(err)
	}
	channels, err = database.ListChannels()
	if err != nil {
		t.Fatal(err)
	}
	if len(channels) != 0 {
		t.Fatalf("expected deleted channel, got %#v", channels)
	}
}

func TestSettingsPersistenceRoundTrip(t *testing.T) {
	database := openTestDatabase(t)
	value := appsettings.AppSettings{
		DownloadDir:            filepath.Join(t.TempDir(), "downloads"),
		Resolution:             appsettings.Resolution1080p,
		OutputFormat:           appsettings.OutputFormatMKV,
		MaxConcurrentDownloads: 3,
		Theme:                  appsettings.ThemeLight,
	}
	record, err := appsettings.NewStorageRecord(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SaveSettings(record); err != nil {
		t.Fatal(err)
	}

	loaded, found, err := database.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected stored settings")
	}
	restored, err := loaded.AppSettings()
	if err != nil {
		t.Fatal(err)
	}
	if restored != appsettings.Normalize(value) {
		t.Fatalf("unexpected settings: %#v", restored)
	}
}

func TestDownloadHistoryPersistenceAndInterruptedRecovery(t *testing.T) {
	database := openTestDatabase(t)

	running := downloader.DownloadTask{
		TaskID:      "download-running",
		VideoNo:     1001,
		VideoTitle:  "진행 작업",
		ChannelName: "채널",
		URL:         "https://chzzk.naver.com/video/1001",
		OutputDir:   t.TempDir(),
		Status:      downloader.TaskStatusRunning,
		Progress: downloader.DownloadProgress{
			Status:              "downloading",
			Percent:             42.5,
			DownloadedBytes:     425,
			TotalBytes:          1000,
			TotalBytesEstimated: true,
			SpeedBytesPerSecond: 125.5,
			ETASeconds:          5,
		},
		QueuedAt:  "2026-10-01T00:00:00Z",
		StartedAt: "2026-10-01T00:00:01Z",
	}
	completed := downloader.DownloadTask{
		TaskID:      "download-completed",
		VideoNo:     1002,
		VideoTitle:  "완료 작업",
		ChannelName: "채널",
		URL:         "https://chzzk.naver.com/video/1002",
		OutputDir:   t.TempDir(),
		Status:      downloader.TaskStatusCompleted,
		Progress: downloader.DownloadProgress{
			Status:          "completed",
			Percent:         100,
			DownloadedBytes: 2000,
			TotalBytes:      2000,
		},
		FinalPath:  filepath.Join(t.TempDir(), "done.mp4"),
		QueuedAt:   "2026-10-01T00:01:00Z",
		StartedAt:  "2026-10-01T00:01:01Z",
		FinishedAt: "2026-10-01T00:01:10Z",
	}

	if err := database.UpsertDownloadTask(running); err != nil {
		t.Fatal(err)
	}
	if err := database.UpsertDownloadTask(completed); err != nil {
		t.Fatal(err)
	}
	if err := database.RecoverInterruptedDownloads(); err != nil {
		t.Fatal(err)
	}

	tasks, err := database.ListDownloadTasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
	if tasks[0].Status != downloader.TaskStatusCancelled ||
		tasks[0].FinishedAt == "" ||
		tasks[0].Error == "" ||
		!tasks[0].Progress.TotalBytesEstimated {
		t.Fatalf("running task was not recovered: %#v", tasks[0])
	}
	if tasks[1].Status != downloader.TaskStatusCompleted ||
		tasks[1].FinalPath != completed.FinalPath {
		t.Fatalf("completed task changed during recovery: %#v", tasks[1])
	}
}


func TestDeleteDownloadTaskRemovesHistory(t *testing.T) {
	database := openTestDatabase(t)
	task := downloader.DownloadTask{
		TaskID:      "download-delete",
		VideoNo:     2001,
		VideoTitle:  "삭제 작업",
		ChannelName: "채널",
		URL:         "https://chzzk.naver.com/video/2001",
		OutputDir:   t.TempDir(),
		Status:      downloader.TaskStatusCompleted,
		QueuedAt:    "2026-10-01T00:00:00Z",
		FinishedAt:  "2026-10-01T00:01:00Z",
	}
	if err := database.UpsertDownloadTask(task); err != nil {
		t.Fatal(err)
	}
	if err := database.DeleteDownloadTask(task.TaskID); err != nil {
		t.Fatal(err)
	}

	tasks, err := database.ListDownloadTasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 0 {
		t.Fatalf("expected deleted history, got %#v", tasks)
	}
}
