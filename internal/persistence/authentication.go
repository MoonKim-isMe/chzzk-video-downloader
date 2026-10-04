package persistence

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	appsettings "github.com/MoonKim-isMe/chzzk-video-downloader/internal/settings"
)

func (d *Database) LoadAuthenticationSettings() (appsettings.AuthenticationSettings, bool, error) {
	var value appsettings.AuthenticationSettings
	var enabled int
	err := d.db.QueryRow(`SELECT enabled, cookies_file_path
		FROM authentication_settings WHERE id = 1`).Scan(
		&enabled, &value.CookiesFilePath,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return appsettings.AuthenticationSettings{}, false, nil
	}
	if err != nil {
		return appsettings.AuthenticationSettings{}, false, fmt.Errorf("인증 설정을 SQLite에서 조회할 수 없습니다: %w", err)
	}
	value.Enabled = enabled != 0
	value = appsettings.NormalizeAuthentication(value)

	if value.Enabled && (value.CookiesFilePath == "" || value.CookiesFilePath == ".") {
		value.Enabled = false
		value.CookiesFilePath = ""
		if err := d.SaveAuthenticationSettings(value); err != nil {
			return appsettings.AuthenticationSettings{}, false, fmt.Errorf(
				"잘못된 인증 설정을 복구할 수 없습니다: %w",
				err,
			)
		}
	}

	if err := appsettings.ValidateAuthentication(value); err != nil {
		return appsettings.AuthenticationSettings{}, false, err
	}
	return value, true, nil
}

func (d *Database) SaveAuthenticationSettings(value appsettings.AuthenticationSettings) error {
	value = appsettings.NormalizeAuthentication(value)
	if err := appsettings.ValidateAuthentication(value); err != nil {
		return err
	}
	_, err := d.db.Exec(`INSERT INTO authentication_settings (
		id, enabled, cookies_file_path, updated_at
	) VALUES (1, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		enabled = excluded.enabled,
		cookies_file_path = excluded.cookies_file_path,
		updated_at = excluded.updated_at`,
		boolInt(value.Enabled), value.CookiesFilePath, time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("인증 설정을 SQLite에 저장할 수 없습니다: %w", err)
	}
	return nil
}
