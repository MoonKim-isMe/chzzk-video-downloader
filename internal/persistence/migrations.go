package persistence

import (
	"fmt"
	"time"
)

type migration struct {
	version    int
	statements []string
}

var migrations = []migration{
	{
		version: 1,
		statements: []string{
			`CREATE TABLE IF NOT EXISTS saved_channels (
				channel_id TEXT PRIMARY KEY,
				channel_name TEXT NOT NULL,
				channel_image_url TEXT NOT NULL DEFAULT '',
				channel_description TEXT NOT NULL DEFAULT '',
				follower_count INTEGER NOT NULL DEFAULT 0,
				verified_mark INTEGER NOT NULL DEFAULT 0,
				open_live INTEGER NOT NULL DEFAULT 0,
				channel_url TEXT NOT NULL,
				saved_at TEXT NOT NULL
			)`,
			`CREATE TABLE IF NOT EXISTS download_tasks (
				task_id TEXT PRIMARY KEY,
				video_no INTEGER NOT NULL,
				video_title TEXT NOT NULL,
				channel_name TEXT NOT NULL DEFAULT '',
				thumbnail_image_url TEXT NOT NULL DEFAULT '',
				url TEXT NOT NULL,
				output_dir TEXT NOT NULL,
				status TEXT NOT NULL,
				progress_status TEXT NOT NULL DEFAULT '',
				progress_percent REAL NOT NULL DEFAULT 0,
				downloaded_bytes INTEGER NOT NULL DEFAULT 0,
				total_bytes INTEGER NOT NULL DEFAULT 0,
				total_bytes_estimated INTEGER NOT NULL DEFAULT 0,
				speed_bytes_per_second REAL NOT NULL DEFAULT 0,
				eta_seconds INTEGER NOT NULL DEFAULT 0,
				final_path TEXT NOT NULL DEFAULT '',
				error TEXT NOT NULL DEFAULT '',
				queued_at TEXT NOT NULL,
				started_at TEXT NOT NULL DEFAULT '',
				finished_at TEXT NOT NULL DEFAULT '',
				updated_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS idx_download_tasks_queued_at
				ON download_tasks(queued_at, task_id)`,
			`CREATE TABLE IF NOT EXISTS app_settings (
				id INTEGER PRIMARY KEY CHECK (id = 1),
				schema_version INTEGER NOT NULL,
				download_dir TEXT NOT NULL,
				resolution TEXT NOT NULL,
				output_format TEXT NOT NULL,
				max_concurrent_downloads INTEGER NOT NULL,
				updated_at TEXT NOT NULL
			)`,
		},
	},
	{
		version: 2,
		statements: []string{
			`ALTER TABLE app_settings ADD COLUMN theme TEXT NOT NULL DEFAULT 'dark'`,
			`UPDATE app_settings
				SET schema_version = 2,
					theme = CASE
						WHEN theme IN ('light', 'dark') THEN theme
						ELSE 'dark'
					END
				WHERE id = 1`,
		},
	},
	{
		version: 3,
		statements: []string{
			`ALTER TABLE download_tasks ADD COLUMN error_code TEXT NOT NULL DEFAULT ''`,
			`UPDATE download_tasks
				SET error_code = 'partial_data_conflict'
				WHERE status = 'failed'
					AND error IN (
						'이전 다운로드의 임시 데이터와 충돌했습니다. 다운로드를 처음부터 다시 시도해 주세요.',
						'이전 다운로드의 임시 데이터와 충돌했습니다. 임시 파일을 정리한 뒤 다시 시도해 주세요.'
					)`,
			`UPDATE download_tasks
				SET error_code = 'authentication_required'
				WHERE status = 'failed'
					AND error = '로그인이 필요한 콘텐츠입니다. 연령 제한 또는 접근 권한이 필요한 영상일 수 있습니다.'`,
		},
	},
	{
		version: 4,
		statements: []string{
			`ALTER TABLE download_tasks ADD COLUMN log_path TEXT NOT NULL DEFAULT ''`,
		},
	},
}

func (d *Database) migrate() error {
	if _, err := d.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("SQLite migration 테이블을 만들 수 없습니다: %w", err)
	}

	current, err := d.MigrationVersion()
	if err != nil {
		return err
	}

	for _, item := range migrations {
		if item.version <= current {
			continue
		}
		if err := d.applyMigration(item); err != nil {
			return err
		}
		current = item.version
	}
	return nil
}

func (d *Database) applyMigration(item migration) error {
	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("SQLite migration %d 트랜잭션을 시작할 수 없습니다: %w", item.version, err)
	}

	rollback := func(cause error) error {
		_ = tx.Rollback()
		return cause
	}

	for _, statement := range item.statements {
		if _, err := tx.Exec(statement); err != nil {
			return rollback(fmt.Errorf("SQLite migration %d 적용에 실패했습니다: %w", item.version, err))
		}
	}

	if _, err := tx.Exec(
		"INSERT INTO schema_migrations(version, applied_at) VALUES (?, ?)",
		item.version,
		time.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		return rollback(fmt.Errorf("SQLite migration %d 버전을 기록할 수 없습니다: %w", item.version, err))
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("SQLite migration %d을 완료할 수 없습니다: %w", item.version, err)
	}
	return nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func scanBool(value int) bool {
	return value != 0
}

