package videorepair

import (
	"context"
	"fmt"
	"testing"
)

func TestApplyDeepProgressLine(t *testing.T) {
	state := deepProgressState{}
	for _, line := range []string{
		"out_time_us=60000000",
		"speed=2.50x",
		"progress=continue",
	} {
		applyDeepProgressLine(&state, line)
	}
	if state.processedSeconds != 60 {
		t.Fatalf("unexpected processed seconds: %f", state.processedSeconds)
	}
	if state.speed != 2.5 {
		t.Fatalf("unexpected speed: %f", state.speed)
	}
	if state.ended {
		t.Fatal("progress must not be ended")
	}
	if !applyDeepProgressLine(&state, "progress=end") || !state.ended {
		t.Fatal("progress=end must emit and mark completion")
	}
}

func TestDeepDecodeCompletedRequiresDurationCoverage(t *testing.T) {
	if !deepDecodeCompleted(8, 8, true, nil) {
		t.Fatal("full duration must be completed")
	}
	if deepDecodeCompleted(5.233333, 8, true, nil) {
		t.Fatal("progress=end before declared duration must not be treated as complete")
	}
	if deepDecodeCompleted(21590, 21600, true, nil) {
		t.Fatal("ten missing seconds on a six-hour video must not be treated as complete")
	}
	if !deepDecodeCompleted(21596, 21600, true, nil) {
		t.Fatal("small container duration tolerance should be accepted")
	}
}

func TestClassifyDeepDiagnosticByCodecAndStream(t *testing.T) {
	tests := []struct {
		name          string
		line          string
		videoCodec    string
		audioCodec    string
		videoErrors   int
		audioErrors   int
	}{
		{name: "h264", line: "[h264 @ 0001] error while decoding MB 10 10", videoCodec: "h264", videoErrors: 1},
		{name: "hevc", line: "[hevc @ 0001] decode error", videoCodec: "hevc", videoErrors: 1},
		{name: "av1", line: "[libdav1d @ 0001] error while decoding", videoCodec: "av1", videoErrors: 1},
		{name: "aac", line: "[aac @ 0001] decode error", audioCodec: "aac", audioErrors: 1},
		{name: "opus", line: "[opus @ 0001] decode error", audioCodec: "opus", audioErrors: 1},
		{name: "stream-index-audio", line: "Error while decoding stream #0:1: Invalid data found", videoCodec: "h264", audioCodec: "aac", audioErrors: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			classification := classifyDeepDiagnostic(test.line, 0, 1, test.videoCodec, test.audioCodec)
			if classification.videoDecodeErrors != test.videoErrors {
				t.Fatalf("unexpected video error count: %#v", classification)
			}
			if classification.audioDecodeErrors != test.audioErrors {
				t.Fatalf("unexpected audio error count: %#v", classification)
			}
		})
	}
}

func TestClassifyDeepCorruptPacketByStream(t *testing.T) {
	video := classifyDeepDiagnostic(
		"[h264 @ 0001] corrupt packet",
		0,
		1,
		"h264",
		"aac",
	)
	if video.videoCorruptPackets != 1 || video.audioCorruptPackets != 0 {
		t.Fatalf("unexpected video corrupt packet classification: %#v", video)
	}

	audio := classifyDeepDiagnostic(
		"[aac @ 0001] corrupt packet",
		0,
		1,
		"h264",
		"aac",
	)
	if audio.audioCorruptPackets != 1 || audio.videoCorruptPackets != 0 {
		t.Fatalf("unexpected audio corrupt packet classification: %#v", audio)
	}
}

func TestClassifyDeepTimestampAndCorruption(t *testing.T) {
	classification := classifyDeepDiagnostic(
		"[h264 @ 0001] corrupt frame pts_time:12.500 non-monotonous DTS",
		0,
		1,
		"h264",
		"aac",
	)
	if !classification.relevant {
		t.Fatal("expected relevant diagnostic")
	}
	if classification.corruptFrames != 1 || classification.nonMonotonicDTS != 1 || classification.dtsErrors != 1 {
		t.Fatalf("unexpected classification: %#v", classification)
	}
	position, ok := extractDeepDiagnosticTime("error pts_time:12.500")
	if !ok || position != 12.5 {
		t.Fatalf("unexpected diagnostic time: %f / %v", position, ok)
	}
}

func TestDeepDamageAccumulatorMergesAndBoundsRanges(t *testing.T) {
	accumulator := deepDamageAccumulator{}
	accumulator.add(10, "영상 디코딩", 1)
	accumulator.add(11, "영상 디코딩", 2)
	if len(accumulator.ranges) != 1 || accumulator.ranges[0].ErrorCount != 3 {
		t.Fatalf("adjacent ranges were not merged: %#v", accumulator.ranges)
	}

	for index := 0; index < maxDeepDamageRanges+20; index++ {
		accumulator.add(float64(index*10+100), fmt.Sprintf("category-%d", index), 1)
	}
	if len(accumulator.ranges) > maxDeepDamageRanges {
		t.Fatalf("damage ranges are unbounded: %d", len(accumulator.ranges))
	}
}

func TestBuildDeepInspectionNormal(t *testing.T) {
	file, probe := deepFixture()
	result := buildDeepInspectionResult(file, probe, MoovAtomPresent, "", deepDecodeSummary{
		ProcessedSeconds: 120,
		Completed:        true,
	})
	if result.Status != InspectionStatusNormal || result.Repairability != RepairabilityNotNeeded {
		t.Fatalf("unexpected normal result: %#v", result)
	}
	if result.Mode != InspectionModeDeep {
		t.Fatalf("unexpected mode: %s", result.Mode)
	}
}

func TestBuildDeepInspectionTimestampWarning(t *testing.T) {
	file, probe := deepFixture()
	result := buildDeepInspectionResult(file, probe, MoovAtomPresent, "", deepDecodeSummary{
		ProcessedSeconds:     120,
		Completed:            true,
		DTSErrors:            2,
		NonMonotonicDTSCount: 1,
		DamageRanges: []DamageRange{
			{StartSeconds: 50, EndSeconds: 51, Category: "타임스탬프", ErrorCount: 2},
		},
	})
	if result.Status != InspectionStatusWarning || result.Repairability != RepairabilityLossless {
		t.Fatalf("unexpected timestamp result: %#v", result)
	}
	if result.Recommendation.Strategy != RepairStrategyTimestampRemux {
		t.Fatalf("unexpected recommendation: %#v", result.Recommendation)
	}
}

func TestBuildDeepInspectionTruncatedDecode(t *testing.T) {
	file, probe := deepFixture()
	result := buildDeepInspectionResult(file, probe, MoovAtomPresent, "", deepDecodeSummary{
		ProcessedSeconds: 70,
		ProcessFailed:    true,
		Completed:        false,
	})
	if result.Status != InspectionStatusDamaged || result.Repairability != RepairabilityPartial {
		t.Fatalf("unexpected truncated result: %#v", result)
	}
	if result.Recommendation.Strategy != RepairStrategyTruncate {
		t.Fatalf("expected truncate strategy: %#v", result.Recommendation)
	}
	if result.LastHealthySeconds == nil || *result.LastHealthySeconds != 70 {
		t.Fatalf("unexpected last healthy position: %#v", result.LastHealthySeconds)
	}
}

func TestDeepInspectStopsWhenContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := DeepInspect(ctx, "ffprobe", "ffmpeg", "unused.mp4", nil)
	if err == nil || err != context.Canceled {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}

func deepFixture() (FileInfo, quickProbeResponse) {
	probe, _ := parseQuickProbe([]byte(`{
		"streams":[
			{"index":0,"codec_type":"video","codec_name":"h264","width":1920,"height":1080,"avg_frame_rate":"30/1","duration":"120","start_time":"0"},
			{"index":1,"codec_type":"audio","codec_name":"aac","channels":2,"sample_rate":"48000","duration":"120","start_time":"0"}
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
