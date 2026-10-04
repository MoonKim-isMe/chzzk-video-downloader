package persistence

import (
	"database/sql"
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
	if version != 8 {
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
	if version != 8 {
		t.Fatalf("unexpected reopened migration version: %d", version)
	}
}

func TestOpenRepairsLegacySettingsSchemaWithoutDownloadAcceleration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.sqlite3")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}

	for _, statement := range []string{
		`CREATE TABLE schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL
		)`,
		`CREATE TABLE app_settings (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			schema_version INTEGER NOT NULL,
			download_dir TEXT NOT NULL,
			resolution TEXT NOT NULL,
			output_format TEXT NOT NULL,
			max_concurrent_downloads INTEGER NOT NULL,
			updated_at TEXT NOT NULL,
			theme TEXT NOT NULL DEFAULT 'dark'
		)`,
		`INSERT INTO app_settings (
			id,
			schema_version,
			download_dir,
			resolution,
			output_format,
			max_concurrent_downloads,
			updated_at,
			theme
		) VALUES (
			1,
			2,
			'C:\\Downloads',
			'best',
			'mp4',
			3,
			'2026-10-03T00:00:00Z',
			'dark'
		)`,
	} {
		if _, err := raw.Exec(statement); err != nil {
			_ = raw.Close()
			t.Fatal(err)
		}
	}
	for version := 1; version <= 8; version++ {
		if _, err := raw.Exec(
			"INSERT INTO schema_migrations(version, applied_at) VALUES (?, ?)",
			version,
			"2026-10-03T00:00:00Z",
		); err != nil {
			_ = raw.Close()
			t.Fatal(err)
		}
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	database, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	record, found, err := database.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected repaired settings row")
	}
	if record.DownloadAcceleration != appsettings.DownloadAccelerationStandard {
		t.Fatalf("unexpected repaired download acceleration: %q", record.DownloadAcceleration)
	}
	if record.DownloadRateLimitMBps != 0 {
		t.Fatalf("unexpected repaired download rate limit: %v", record.DownloadRateLimitMBps)
	}
	if record.SchemaVersion != appsettings.StorageSchemaVersion {
		t.Fatalf("unexpected repaired settings schema version: %d", record.SchemaVersion)
	}

	var count int
	if err := database.db.QueryRow(`
		SELECT COUNT(*)
		FROM pragma_table_info('app_settings')
		WHERE name = 'download_acceleration'
	`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("download_acceleration column was not repaired: %d", count)
	}

	if err := database.db.QueryRow(`
		SELECT COUNT(*)
		FROM pragma_table_info('app_settings')
		WHERE name = 'download_rate_limit_mbps'
	`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("download_rate_limit_mbps column was not repaired: %d", count)
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
		DownloadRateLimitMBps:   12.5,
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

func TestAuthenticationSettingsPersistenceRoundTrip(t *testing.T) {
	database := openTestDatabase(t)
	value := appsettings.AuthenticationSettings{
		Enabled: true, CookiesFilePath: filepath.Join(t.TempDir(), "cookies.txt"),
	}
	if err := database.SaveAuthenticationSettings(value); err != nil {
		t.Fatal(err)
	}

	loaded, found, err := database.LoadAuthenticationSettings()
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected stored authentication settings")
	}
	if loaded != appsettings.NormalizeAuthentication(value) {
		t.Fatalf("unexpected authentication settings: %#v", loaded)
	}
}

func TestAuthenticationSettingsLoadRepairsEnabledWithoutCookiesFile(t *testing.T) {
	database := openTestDatabase(t)

	if _, err := database.db.Exec(`INSERT INTO authentication_settings (
		id, enabled, cookies_file_path, updated_at
	) VALUES (1, 1, '', '2026-10-04T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}

	loaded, found, err := database.LoadAuthenticationSettings()
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected stored authentication settings")
	}
	if loaded.Enabled || loaded.CookiesFilePath != "" {
		t.Fatalf("unexpected repaired authentication settings: %#v", loaded)
	}

	var enabled int
	var cookiesFilePath string
	if err := database.db.QueryRow(`SELECT enabled, cookies_file_path
		FROM authentication_settings WHERE id = 1`).Scan(&enabled, &cookiesFilePath); err != nil {
		t.Fatal(err)
	}
	if enabled != 0 || cookiesFilePath != "" {
		t.Fatalf("authentication settings were not repaired in SQLite: enabled=%d path=%q", enabled, cookiesFilePath)
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


func TestDownloadHistoryPersistsErrorCode(t *testing.T) {
	database := openTestDatabase(t)
	task := downloader.DownloadTask{
		TaskID:      "download-failed-with-code",
		VideoNo:     3001,
		VideoTitle:  "복구 가능한 실패",
		ChannelName: "채널",
		URL:         "https://chzzk.naver.com/video/3001",
		OutputDir:   t.TempDir(),
		Status:      downloader.TaskStatusFailed,
		Error:       "이전 다운로드의 임시 데이터와 충돌했습니다. 임시 파일을 정리한 뒤 다시 시도해 주세요.",
		ErrorCode:   downloader.DownloadFailurePartialDataConflict,
		LogPath:     filepath.Join(t.TempDir(), "failure.log"),
		QueuedAt:    "2026-10-01T00:00:00Z",
		FinishedAt:  "2026-10-01T00:00:01Z",
	}
	if err := database.UpsertDownloadTask(task); err != nil {
		t.Fatal(err)
	}

	tasks, err := database.ListDownloadTasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected one task, got %#v", tasks)
	}
	if tasks[0].ErrorCode != downloader.DownloadFailurePartialDataConflict {
		t.Fatalf("unexpected restored error code: %q", tasks[0].ErrorCode)
	}
	if tasks[0].LogPath != task.LogPath {
		t.Fatalf("unexpected restored log path: %q", tasks[0].LogPath)
	}
}
