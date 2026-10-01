package settings

import "fmt"

const StorageSchemaVersion = 1

type StorageRecord struct {
	SchemaVersion          int          `json:"schemaVersion"`
	DownloadDir            string       `json:"downloadDir"`
	Resolution             Resolution   `json:"resolution"`
	OutputFormat           OutputFormat `json:"outputFormat"`
	MaxConcurrentDownloads int          `json:"maxConcurrentDownloads"`
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
		MaxConcurrentDownloads: value.MaxConcurrentDownloads,
	}, nil
}

func (record StorageRecord) AppSettings() (AppSettings, error) {
	if record.SchemaVersion != StorageSchemaVersion {
		return AppSettings{}, fmt.Errorf(
			"지원하지 않는 설정 저장 버전입니다: %d",
			record.SchemaVersion,
		)
	}

	value := Normalize(AppSettings{
		DownloadDir:            record.DownloadDir,
		Resolution:             record.Resolution,
		OutputFormat:           record.OutputFormat,
		MaxConcurrentDownloads: record.MaxConcurrentDownloads,
	})
	if err := Validate(value); err != nil {
		return AppSettings{}, err
	}
	return value, nil
}
