package settings

import "fmt"

const StorageSchemaVersion = 3

type StorageRecord struct {
	SchemaVersion          int                  `json:"schemaVersion"`
	DownloadDir            string               `json:"downloadDir"`
	Resolution             Resolution           `json:"resolution"`
	OutputFormat           OutputFormat         `json:"outputFormat"`
	DownloadAcceleration   DownloadAcceleration `json:"downloadAcceleration"`
	MaxConcurrentDownloads int                  `json:"maxConcurrentDownloads"`
	Theme                  ThemeMode            `json:"theme"`
}

func NewStorageRecord(value AppSettings) (StorageRecord, error) {
	value = Normalize(value)
	if err := Validate(value); err != nil {
		return StorageRecord{}, err
	}

	return StorageRecord{
		SchemaVersion:          StorageSchemaVersion,
		DownloadDir:            value.DownloadDir,
		Resolution:             value.Resolution,
		OutputFormat:           value.OutputFormat,
		DownloadAcceleration:   value.DownloadAcceleration,
		MaxConcurrentDownloads: value.MaxConcurrentDownloads,
		Theme:                  value.Theme,
	}, nil
}

func (record StorageRecord) AppSettings() (AppSettings, error) {
	if record.SchemaVersion < 1 || record.SchemaVersion > StorageSchemaVersion {
		return AppSettings{}, fmt.Errorf(
			"지원하지 않는 설정 저장 버전입니다: %d",
			record.SchemaVersion,
		)
	}

	theme := record.Theme
	if record.SchemaVersion == 1 {
		theme = ThemeDark
	}
	downloadAcceleration := record.DownloadAcceleration
	if record.SchemaVersion <= 2 {
		downloadAcceleration = DefaultDownloadAcceleration
	}

	value := Normalize(AppSettings{
		DownloadDir:            record.DownloadDir,
		Resolution:             record.Resolution,
		OutputFormat:           record.OutputFormat,
		DownloadAcceleration:   downloadAcceleration,
		MaxConcurrentDownloads: record.MaxConcurrentDownloads,
		Theme:                  theme,
	})
	if err := Validate(value); err != nil {
		return AppSettings{}, err
	}
	return value, nil
}
