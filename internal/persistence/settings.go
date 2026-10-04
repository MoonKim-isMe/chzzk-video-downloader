package persistence

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	appsettings "github.com/MoonKim-isMe/chzzk-video-downloader/internal/settings"
)

func (d *Database) LoadSettings() (appsettings.StorageRecord, bool, error) {
	var record appsettings.StorageRecord
	err := d.db.QueryRow(`SELECT
		schema_version,
		download_dir,
		resolution,
		output_format,
		download_acceleration,
		download_rate_limit_mbps,
		max_concurrent_downloads,
		theme
	FROM app_settings
	WHERE id = 1`).Scan(
		&record.SchemaVersion,
		&record.DownloadDir,
		&record.Resolution,
		&record.OutputFormat,
		&record.DownloadAcceleration,
		&record.DownloadRateLimitMBps,
		&record.MaxConcurrentDownloads,
		&record.Theme,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return appsettings.StorageRecord{}, false, nil
	}
	if err != nil {
		return appsettings.StorageRecord{}, false, fmt.Errorf("설정을 SQLite에서 조회할 수 없습니다: %w", err)
	}
	return record, true, nil
}

func (d *Database) SaveSettings(record appsettings.StorageRecord) error {
	if _, err := record.AppSettings(); err != nil {
		return err
	}

	_, err := d.db.Exec(`INSERT INTO app_settings (
		id,
		schema_version,
		download_dir,
		resolution,
		output_format,
		download_acceleration,
		download_rate_limit_mbps,
		max_concurrent_downloads,
		theme,
		updated_at
	) VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		schema_version = excluded.schema_version,
		download_dir = excluded.download_dir,
		resolution = excluded.resolution,
		output_format = excluded.output_format,
		download_acceleration = excluded.download_acceleration,
		download_rate_limit_mbps = excluded.download_rate_limit_mbps,
		max_concurrent_downloads = excluded.max_concurrent_downloads,
		theme = excluded.theme,
		updated_at = excluded.updated_at`,
		record.SchemaVersion,
		record.DownloadDir,
		string(record.Resolution),
		string(record.OutputFormat),
		string(record.DownloadAcceleration),
		record.DownloadRateLimitMBps,
		record.MaxConcurrentDownloads,
		string(record.Theme),
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("설정을 SQLite에 저장할 수 없습니다: %w", err)
	}
	return nil
}
