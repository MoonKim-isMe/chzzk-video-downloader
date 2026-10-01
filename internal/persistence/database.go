package persistence

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

const (
	databaseFileName = "data.sqlite3"
	appDataDirName   = "CHZZK Video Downloader"
)

type Database struct {
	db   *sql.DB
	path string
}

func DefaultPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("사용자 설정 디렉터리를 확인할 수 없습니다: %w", err)
	}
	if strings.TrimSpace(configDir) == "" {
		return "", fmt.Errorf("사용자 설정 디렉터리를 확인할 수 없습니다")
	}
	return filepath.Join(configDir, appDataDirName, databaseFileName), nil
}

func Open(path string) (*Database, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("SQLite 데이터베이스 경로가 필요합니다")
	}

	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, fmt.Errorf("SQLite 데이터 디렉터리를 만들 수 없습니다: %w", err)
		}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("SQLite 데이터베이스를 열 수 없습니다: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	cleanup := func(cause error) (*Database, error) {
		_ = db.Close()
		return nil, cause
	}

	for _, statement := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA journal_mode = WAL",
	} {
		if _, err := db.Exec(statement); err != nil {
			return cleanup(fmt.Errorf("SQLite 초기 설정에 실패했습니다: %w", err))
		}
	}

	if err := db.Ping(); err != nil {
		return cleanup(fmt.Errorf("SQLite 연결을 확인할 수 없습니다: %w", err))
	}

	database := &Database{db: db, path: path}
	if err := database.migrate(); err != nil {
		return cleanup(err)
	}
	return database, nil
}

func (d *Database) Path() string {
	if d == nil {
		return ""
	}
	return d.path
}

func (d *Database) Close() error {
	if d == nil || d.db == nil {
		return nil
	}
	return d.db.Close()
}

func (d *Database) MigrationVersion() (int, error) {
	var version int
	err := d.db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("SQLite migration 버전을 확인할 수 없습니다: %w", err)
	}
	return version, nil
}
