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
	{
		version: 5,
		statements: []string{
			`ALTER TABLE app_settings ADD COLUMN download_acceleration TEXT NOT NULL DEFAULT 'standard'`,
			`UPDATE app_settings
				SET schema_version = 3,
					download_acceleration = CASE
						WHEN download_acceleration IN ('stable', 'standard', 'fast', 'ultra') THEN download_acceleration
						ELSE 'standard'
					END
				WHERE id = 1`,
		},
	},
	{
		version: 6,
		statements: []string{
			`UPDATE download_tasks
				SET error_code = 'hls_initialization_fragment_order',
					error = '영상 스트림 구조 문제로 다운로드에 실패했습니다. 새로 다운로드하면 대체 방식으로 자동 재시도합니다.'
				WHERE status = 'failed'
					AND error_code = 'partial_data_conflict'
					AND error IN (
						'이전 다운로드의 임시 데이터와 충돌했습니다. 다운로드를 처음부터 다시 시도해 주세요.',
						'이전 다운로드의 임시 데이터와 충돌했습니다. 임시 파일을 정리한 뒤 다시 시도해 주세요.'
					)`,
		},
	},
	{
		version: 7,
		statements: []string{
			`ALTER TABLE app_settings ADD COLUMN download_rate_limit_mbps REAL NOT NULL DEFAULT 0`,
			`UPDATE app_settings
				SET schema_version = 4,
					download_rate_limit_mbps = CASE
						WHEN download_rate_limit_mbps >= 0 THEN download_rate_limit_mbps
						ELSE 0
					END
				WHERE id = 1`,
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

	if err := d.repairLegacySettingsSchema(); err != nil {
		return err
	}
	return nil
}

func (d *Database) repairLegacySettingsSchema() error {
	var hasDownloadAcceleration int
	if err := d.db.QueryRow(`
		SELECT COUNT(*)
		FROM pragma_table_info('app_settings')
		WHERE name = 'download_acceleration'
	`).Scan(&hasDownloadAcceleration); err != nil {
		return fmt.Errorf("SQLite 설정 스키마를 확인할 수 없습니다: %w", err)
	}
	if hasDownloadAcceleration == 0 {
		if _, err := d.db.Exec(
			`ALTER TABLE app_settings
				ADD COLUMN download_acceleration TEXT NOT NULL DEFAULT 'standard'`,
		); err != nil {
			return fmt.Errorf("SQLite 다운로드 가속 설정 컬럼을 복구할 수 없습니다: %w", err)
		}
	}

	var hasDownloadRateLimit int
	if err := d.db.QueryRow(`
		SELECT COUNT(*)
		FROM pragma_table_info('app_settings')
		WHERE name = 'download_rate_limit_mbps'
	`).Scan(&hasDownloadRateLimit); err != nil {
		return fmt.Errorf("SQLite 설정 스키마를 확인할 수 없습니다: %w", err)
	}
	if hasDownloadRateLimit == 0 {
		if _, err := d.db.Exec(
			`ALTER TABLE app_settings
				ADD COLUMN download_rate_limit_mbps REAL NOT NULL DEFAULT 0`,
		); err != nil {
			return fmt.Errorf("SQLite 다운로드 속도 제한 컬럼을 복구할 수 없습니다: %w", err)
		}
	}

	if _, err := d.db.Exec(`UPDATE app_settings
		SET schema_version = CASE
				WHEN schema_version < 4 THEN 4
				ELSE schema_version
			END,
			download_acceleration = CASE
				WHEN download_acceleration IN ('stable', 'standard', 'fast', 'ultra')
					THEN download_acceleration
				ELSE 'standard'
			END,
			download_rate_limit_mbps = CASE
				WHEN download_rate_limit_mbps >= 0 THEN download_rate_limit_mbps
				ELSE 0
			END
		WHERE id = 1`); err != nil {
		return fmt.Errorf("SQLite 다운로드 설정을 복구할 수 없습니다: %w", err)
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

