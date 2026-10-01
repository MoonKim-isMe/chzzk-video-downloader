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
		MaxConcurrentDownloads: 3,
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
