package videorepair

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestQuickSamplePointsLongVideoUsesStartMiddleEnd(t *testing.T) {
	points := quickSamplePoints(6 * 60 * 60)
	if len(points) != 3 {
		t.Fatalf("expected three sample points, got %#v", points)
	}
	if points[0] != 0 {
		t.Fatalf("first sample must start at zero: %#v", points)
	}
	if points[1] < 10000 || points[1] > 12000 {
		t.Fatalf("middle sample is not near the middle: %#v", points)
	}
	if points[2] < 21590 {
		t.Fatalf("end sample is not near the end: %#v", points)
	}
}

func TestParseQuickProbeAndBuildStreamMetadata(t *testing.T) {
	probe, err := parseQuickProbe([]byte(`{
		"streams":[
			{"codec_type":"video","codec_name":"h264","profile":"High","width":1920,"height":1080,"avg_frame_rate":"60000/1001","duration":"120.500","start_time":"0.000"},
			{"codec_type":"audio","codec_name":"aac","channels":2,"sample_rate":"48000","duration":"120.480","start_time":"0.021"}
		],
		"format":{"format_name":"mov,mp4,m4a,3gp,3g2,mj2","duration":"120.500","start_time":"0.000"}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	file := FileInfo{Extension: ".mp4", Container: "MP4", ContainerSource: "extension"}
	applyProbeFileInfo(&file, probe)
	video := buildVideoInspection(probe.Streams)
	audio := buildAudioInspection(probe.Streams)
	if !file.MetadataAvailable || file.DurationSeconds != 120.5 {
		t.Fatalf("unexpected file metadata: %#v", file)
	}
	if video.Codec != "h264" || video.Width != 1920 || video.Height != 1080 || video.FPS < 59 {
		t.Fatalf("unexpected video metadata: %#v", video)
	}
	if audio.Codec != "aac" || audio.Channels != 2 || audio.SampleRate != 48000 {
		t.Fatalf("unexpected audio metadata: %#v", audio)
	}
}

func TestInspectMoovAtomFixture(t *testing.T) {
	withMoov := filepath.Join(t.TempDir(), "with-moov.mp4")
	if err := os.WriteFile(withMoov, appendAtom(appendAtom(nil, "ftyp", nil), "moov", nil), 0o644); err != nil {
		t.Fatal(err)
	}
	if status := inspectMoovAtom(withMoov, ".mp4"); status != MoovAtomPresent {
		t.Fatalf("expected moov atom, got %s", status)
	}

	withoutMoov := filepath.Join(t.TempDir(), "without-moov.mp4")
	if err := os.WriteFile(withoutMoov, appendAtom(nil, "ftyp", nil), 0o644); err != nil {
		t.Fatal(err)
	}
	if status := inspectMoovAtom(withoutMoov, ".mp4"); status != MoovAtomMissing {
		t.Fatalf("expected missing moov atom, got %s", status)
	}
}

func TestBuildQuickInspectionNormalFixture(t *testing.T) {
	file, probe := quickFixture()
	result := buildQuickInspectionResult(file, probe, MoovAtomPresent, []quickSampleResult{
		{StartSeconds: 0, EndSeconds: 2},
		{StartSeconds: 59, EndSeconds: 61},
		{StartSeconds: 118, EndSeconds: 120},
	})
	if result.Status != InspectionStatusNormal || result.Repairability != RepairabilityNotNeeded {
		t.Fatalf("unexpected normal result: %#v", result)
	}
	if result.LastHealthySeconds == nil || *result.LastHealthySeconds != file.DurationSeconds {
		t.Fatalf("expected healthy end position: %#v", result.LastHealthySeconds)
	}
}

func TestBuildQuickInspectionTimestampWarningFixture(t *testing.T) {
	file, probe := quickFixture()
	result := buildQuickInspectionResult(file, probe, MoovAtomPresent, []quickSampleResult{
		{StartSeconds: 59, EndSeconds: 61, HadError: true, DTSErrors: 1, NonMonotonicDTS: 1},
	})
	if result.Status != InspectionStatusWarning {
		t.Fatalf("expected warning result: %#v", result)
	}
	if result.Repairability != RepairabilityLossless || result.Recommendation.Strategy != RepairStrategyTimestampRemux {
		t.Fatalf("expected timestamp remux recommendation: %#v", result.Recommendation)
	}
}

func TestApplyQuickProbeWarningsRecommendsDeepInspection(t *testing.T) {
	file, probe := quickFixture()
	result := buildQuickInspectionResult(file, probe, MoovAtomPresent, []quickSampleResult{
		{StartSeconds: 0, EndSeconds: 2},
	})
	applyQuickProbeWarnings(&result, "stream 0: duration warning")
	if result.Status != InspectionStatusWarning {
		t.Fatalf("expected warning result: %#v", result)
	}
	if !result.DeepInspectionRecommended || result.DeepInspectionReason == "" {
		t.Fatalf("expected deep inspection recommendation: %#v", result)
	}
}

func TestBuildQuickInspectionDecodeDamageFixture(t *testing.T) {
	file, probe := quickFixture()
	result := buildQuickInspectionResult(file, probe, MoovAtomPresent, []quickSampleResult{
		{StartSeconds: 59, EndSeconds: 61, HadError: true, VideoDecodeErrors: 2, CorruptFrames: 1},
	})
	if result.Status != InspectionStatusDamaged {
		t.Fatalf("expected damaged result: %#v", result)
	}
	if result.Repairability != RepairabilityPartial || len(result.DamageRanges) != 1 {
		t.Fatalf("expected partial repair range: %#v", result)
	}
}

func TestQuickInspectStopsWhenContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := QuickInspect(ctx, "ffprobe", "ffmpeg", "unused.mp4", nil)
	if err == nil || err != context.Canceled {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}

func quickFixture() (FileInfo, quickProbeResponse) {
	probe, _ := parseQuickProbe([]byte(`{
		"streams":[
			{"codec_type":"video","codec_name":"h264","width":1920,"height":1080,"avg_frame_rate":"30/1","duration":"120","start_time":"0"},
			{"codec_type":"audio","codec_name":"aac","channels":2,"sample_rate":"48000","duration":"120","start_time":"0"}
		],
		"format":{"format_name":"mov,mp4,m4a,3gp,3g2,mj2","duration":"120","start_time":"0"}
	}`))
	file := FileInfo{
		Path:              "sample.mp4",
		Name:              "sample.mp4",
		SizeBytes:         1024,
		Extension:         ".mp4",
		Container:         "MP4",
		ContainerSource:   "probe",
		FormatName:        probe.Format.FormatName,
		DurationSeconds:   120,
		MetadataAvailable: true,
	}
	return file, probe
}

func appendAtom(buffer []byte, atomType string, payload []byte) []byte {
	size := uint32(8 + len(payload))
	header := make([]byte, 8)
	binary.BigEndian.PutUint32(header[:4], size)
	copy(header[4:8], []byte(atomType))
	buffer = append(buffer, header...)
	return append(buffer, payload...)
}
