package settings

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestStorageRecordRoundTrip(t *testing.T) {
	value := AppSettings{
		DownloadDir:            filepath.Join(t.TempDir(), "video"),
		Resolution:             Resolution1440p,
		OutputFormat:           OutputFormatMKV,
		DownloadAcceleration:   DownloadAccelerationUltra,
		DownloadRateLimitMBps:   12.5,
		MaxConcurrentDownloads: 3,
		Theme:                  ThemeLight,
	}

	record, err := NewStorageRecord(value)
	if err != nil {
		t.Fatal(err)
	}
	if record.SchemaVersion != StorageSchemaVersion {
		t.Fatalf("unexpected schema version: %d", record.SchemaVersion)
	}

	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}

	var decoded StorageRecord
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}

	restored, err := decoded.AppSettings()
	if err != nil {
		t.Fatal(err)
	}
	if restored != Normalize(value) {
		t.Fatalf("round trip mismatch: got %#v want %#v", restored, Normalize(value))
	}
}

func TestStorageRecordRejectsUnsupportedVersion(t *testing.T) {
	record := StorageRecord{
		SchemaVersion:          StorageSchemaVersion + 1,
		DownloadDir:            t.TempDir(),
		Resolution:             ResolutionBest,
		OutputFormat:           OutputFormatMP4,
		MaxConcurrentDownloads: 1,
	}
	if _, err := record.AppSettings(); err == nil {
		t.Fatal("expected unsupported version error")
	}
}

func TestStorageRecordRejectsInvalidSettings(t *testing.T) {
	record := StorageRecord{
		SchemaVersion:          StorageSchemaVersion,
		DownloadDir:            t.TempDir(),
		Resolution:             "480p",
		OutputFormat:           OutputFormatMP4,
		MaxConcurrentDownloads: 1,
	}
	if _, err := record.AppSettings(); err == nil {
		t.Fatal("expected invalid stored settings error")
	}
}


func TestStorageRecordV1DefaultsToDarkTheme(t *testing.T) {
	record := StorageRecord{
		SchemaVersion:          1,
		DownloadDir:            t.TempDir(),
		Resolution:             ResolutionBest,
		OutputFormat:           OutputFormatMP4,
		MaxConcurrentDownloads: 1,
	}

	value, err := record.AppSettings()
	if err != nil {
		t.Fatal(err)
	}
	if value.Theme != ThemeDark {
		t.Fatalf("legacy theme should default to dark: %#v", value)
	}
	if value.DownloadAcceleration != DefaultDownloadAcceleration {
		t.Fatalf("legacy acceleration should use default: %#v", value)
	}
	if value.DownloadRateLimitMBps != DefaultDownloadRateLimitMBps {
		t.Fatalf("legacy rate limit should use default: %#v", value)
	}
}

func TestStorageRecordV2DefaultsDownloadAcceleration(t *testing.T) {
	record := StorageRecord{
		SchemaVersion:          2,
		DownloadDir:            t.TempDir(),
		Resolution:             ResolutionBest,
		OutputFormat:           OutputFormatMP4,
		MaxConcurrentDownloads: 2,
		Theme:                  ThemeLight,
	}

	value, err := record.AppSettings()
	if err != nil {
		t.Fatal(err)
	}
	if value.Theme != ThemeLight {
		t.Fatalf("v2 theme should be preserved: %#v", value)
	}
	if value.DownloadAcceleration != DefaultDownloadAcceleration {
		t.Fatalf("v2 acceleration should use default: %#v", value)
	}
	if value.DownloadRateLimitMBps != DefaultDownloadRateLimitMBps {
		t.Fatalf("v2 rate limit should use default: %#v", value)
	}
}

func TestStorageRecordV3DefaultsDownloadRateLimit(t *testing.T) {
	record := StorageRecord{
		SchemaVersion:          3,
		DownloadDir:            t.TempDir(),
		Resolution:             ResolutionBest,
		OutputFormat:           OutputFormatMP4,
		DownloadAcceleration:   DownloadAccelerationFast,
		DownloadRateLimitMBps:   99,
		MaxConcurrentDownloads: 2,
		Theme:                  ThemeLight,
	}

	value, err := record.AppSettings()
	if err != nil {
		t.Fatal(err)
	}
	if value.DownloadAcceleration != DownloadAccelerationFast {
		t.Fatalf("v3 acceleration should be preserved: %#v", value)
	}
	if value.DownloadRateLimitMBps != DefaultDownloadRateLimitMBps {
		t.Fatalf("v3 rate limit should use default: %#v", value)
	}
}
