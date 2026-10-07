package videorepair

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func float64Pointer(value float64) *float64 {
	return &value
}

func validInspectionFixture() InspectionResult {
	return InspectionResult{
		Mode:          InspectionModeDeep,
		Status:        InspectionStatusDamaged,
		Repairability: RepairabilityLossless,
		Summary:       "일부 타임스탬프 오류가 감지되었습니다.",
		InspectedAt:   time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC),
		File: FileInfo{
			Path:              "C:/video/sample.mp4",
			Name:              "sample.mp4",
			SizeBytes:         1024,
			Extension:         ".mp4",
			Container:         "MP4",
			ContainerSource:   "probe",
			DurationSeconds:   120,
			MetadataAvailable: true,
		},
		Container: ContainerInspection{
			Status:                 HealthStatusWarning,
			Parseable:              true,
			StreamMetadataReadable: true,
			IndexStatus:            HealthStatusNormal,
			MoovAtomStatus:         MoovAtomPresent,
		},
		Video: VideoStreamInspection{
			Status:          HealthStatusNormal,
			Present:         true,
			Codec:           "h264",
			Width:           1920,
			Height:          1080,
			FPS:             60,
			DurationSeconds: 120,
		},
		Audio: AudioStreamInspection{
			Status:          HealthStatusNormal,
			Present:         true,
			Codec:           "aac",
			Channels:        2,
			SampleRate:      48000,
			DurationSeconds: 120,
		},
		Timestamps: TimestampInspection{
			Status:        HealthStatusWarning,
			PTSErrorCount: 2,
		},
		AVSync: AVSyncInspection{
			Status:                    HealthStatusNormal,
			VideoDurationSeconds:      120,
			AudioDurationSeconds:      120,
			DurationDifferenceSeconds: 0,
		},
		FirstErrorSeconds:  float64Pointer(62.5),
		LastErrorSeconds:   float64Pointer(62.8),
		LastHealthySeconds: float64Pointer(120),
		DamageRanges: []DamageRange{
			{StartSeconds: 62.5, EndSeconds: 62.8, Category: "timestamp", ErrorCount: 2},
		},
		Recommendation: RepairRecommendation{
			Strategy: RepairStrategyTimestampRemux,
			Summary:  "타임스탬프를 정규화한 뒤 무손실 Remux를 권장합니다.",
		},
		Diagnostics: DiagnosticReferences{
			FFprobeLogPath: "C:/logs/ffprobe.log",
			FFmpegLogPath:  "C:/logs/ffmpeg.log",
		},
	}
}

func TestInspectionResultValidate(t *testing.T) {
	result := validInspectionFixture()
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestInspectionResultRejectsInvalidDamageRange(t *testing.T) {
	result := validInspectionFixture()
	result.DamageRanges[0].EndSeconds = 10
	if err := result.Validate(); err == nil {
		t.Fatal("expected invalid damage range")
	}
}

func TestInspectionResultRejectsUnknownStatus(t *testing.T) {
	result := validInspectionFixture()
	result.Status = InspectionStatus("unexpected")
	if err := result.Validate(); err == nil {
		t.Fatal("expected invalid inspection status")
	}
}

func TestInspectionResultJSONDoesNotEmbedRawDiagnosticOutput(t *testing.T) {
	result := validInspectionFixture()
	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(payload)
	if !strings.Contains(serialized, "ffprobeLogPath") || !strings.Contains(serialized, "ffmpegLogPath") {
		t.Fatalf("diagnostic references are missing: %s", serialized)
	}
	for _, rawField := range []string{"stderr", "stdout", "rawLog", "rawOutput"} {
		if strings.Contains(serialized, rawField) {
			t.Fatalf("raw diagnostic field must not be serialized: %s", rawField)
		}
	}
}
