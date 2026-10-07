package videorepair

import (
	"fmt"
	"strings"
	"time"
)

type InspectionMode string

const (
	InspectionModeQuick InspectionMode = "quick"
	InspectionModeDeep  InspectionMode = "deep"
)

type InspectionStatus string

const (
	InspectionStatusNormal  InspectionStatus = "normal"
	InspectionStatusWarning InspectionStatus = "warning"
	InspectionStatusDamaged InspectionStatus = "damaged"
	InspectionStatusFailed  InspectionStatus = "failed"
)

type Repairability string

const (
	RepairabilityNotNeeded  Repairability = "not_needed"
	RepairabilityLossless   Repairability = "lossless"
	RepairabilityPartial    Repairability = "partial"
	RepairabilityReencode   Repairability = "reencode"
	RepairabilityImpossible Repairability = "impossible"
)

type HealthStatus string

const (
	HealthStatusUnknown     HealthStatus = "unknown"
	HealthStatusNormal      HealthStatus = "normal"
	HealthStatusWarning     HealthStatus = "warning"
	HealthStatusDamaged     HealthStatus = "damaged"
	HealthStatusUnavailable HealthStatus = "unavailable"
)

type RepairStrategy string

const (
	RepairStrategyNone                RepairStrategy = "none"
	RepairStrategyCompatibilityRemux  RepairStrategy = "compatibility_remux"
	RepairStrategyRemux               RepairStrategy = "remux"
	RepairStrategyTimestampRemux  RepairStrategy = "timestamp_remux"
	RepairStrategyPartial         RepairStrategy = "partial"
	RepairStrategyTruncate        RepairStrategy = "truncate"
	RepairStrategyReencode        RepairStrategy = "reencode"
	RepairStrategyUnavailable     RepairStrategy = "unavailable"
)

type MoovAtomStatus string

const (
	MoovAtomNotApplicable MoovAtomStatus = "not_applicable"
	MoovAtomPresent       MoovAtomStatus = "present"
	MoovAtomMissing       MoovAtomStatus = "missing"
	MoovAtomUnknown       MoovAtomStatus = "unknown"
)

type ContainerInspection struct {
	Status                 HealthStatus   `json:"status"`
	Parseable              bool           `json:"parseable"`
	StreamMetadataReadable bool           `json:"streamMetadataReadable"`
	IndexStatus            HealthStatus   `json:"indexStatus"`
	MoovAtomStatus         MoovAtomStatus `json:"moovAtomStatus"`
	ErrorCount             int            `json:"errorCount"`
}

type VideoStreamInspection struct {
	Status             HealthStatus `json:"status"`
	Present            bool         `json:"present"`
	Codec              string       `json:"codec,omitempty"`
	Profile            string       `json:"profile,omitempty"`
	Width              int          `json:"width,omitempty"`
	Height             int          `json:"height,omitempty"`
	FPS                float64      `json:"fps,omitempty"`
	DurationSeconds    float64      `json:"durationSeconds,omitempty"`
	DecodeErrorCount   int          `json:"decodeErrorCount"`
	CorruptFrameCount  int          `json:"corruptFrameCount"`
	CorruptPacketCount int          `json:"corruptPacketCount"`
}

type AudioStreamInspection struct {
	Status             HealthStatus `json:"status"`
	Present            bool         `json:"present"`
	Codec              string       `json:"codec,omitempty"`
	Channels           int          `json:"channels,omitempty"`
	SampleRate         int          `json:"sampleRate,omitempty"`
	DurationSeconds    float64      `json:"durationSeconds,omitempty"`
	DecodeErrorCount   int          `json:"decodeErrorCount"`
	CorruptPacketCount int          `json:"corruptPacketCount"`
}

type TimestampInspection struct {
	Status               HealthStatus `json:"status"`
	PTSErrorCount        int          `json:"ptsErrorCount"`
	DTSErrorCount        int          `json:"dtsErrorCount"`
	NonMonotonicDTSCount int          `json:"nonMonotonicDtsCount"`
	JumpCount            int          `json:"jumpCount"`
}

type AVSyncInspection struct {
	Status                    HealthStatus `json:"status"`
	VideoDurationSeconds      float64      `json:"videoDurationSeconds,omitempty"`
	AudioDurationSeconds      float64      `json:"audioDurationSeconds,omitempty"`
	DurationDifferenceSeconds float64      `json:"durationDifferenceSeconds,omitempty"`
	StartDifferenceSeconds    float64      `json:"startDifferenceSeconds,omitempty"`
}

type DamageRange struct {
	StartSeconds float64 `json:"startSeconds"`
	EndSeconds   float64 `json:"endSeconds"`
	Category     string  `json:"category"`
	ErrorCount   int     `json:"errorCount"`
}

type RepairRecommendation struct {
	Strategy    RepairStrategy `json:"strategy"`
	Summary     string         `json:"summary"`
	QualityLoss bool           `json:"qualityLoss"`
	SegmentLoss bool           `json:"segmentLoss"`
}

type DiagnosticReferences struct {
	FFprobeLogPath string `json:"ffprobeLogPath,omitempty"`
	FFmpegLogPath  string `json:"ffmpegLogPath,omitempty"`
}

type InspectionResult struct {
	Mode                 InspectionMode        `json:"mode"`
	Status               InspectionStatus      `json:"status"`
	Repairability        Repairability         `json:"repairability"`
	Summary              string                `json:"summary"`
	InspectedAt          time.Time             `json:"inspectedAt"`
	File                 FileInfo              `json:"file"`
	Container            ContainerInspection   `json:"container"`
	Video                VideoStreamInspection `json:"video"`
	Audio                AudioStreamInspection `json:"audio"`
	Timestamps           TimestampInspection   `json:"timestamps"`
	AVSync               AVSyncInspection      `json:"avSync"`
	FirstErrorSeconds    *float64              `json:"firstErrorSeconds,omitempty"`
	LastErrorSeconds     *float64              `json:"lastErrorSeconds,omitempty"`
	LastHealthySeconds   *float64              `json:"lastHealthySeconds,omitempty"`
	DamageRanges         []DamageRange         `json:"damageRanges"`
	Recommendation            RepairRecommendation `json:"recommendation"`
	DeepInspectionRecommended bool                 `json:"deepInspectionRecommended"`
	DeepInspectionReason      string               `json:"deepInspectionReason,omitempty"`
	Diagnostics               DiagnosticReferences `json:"diagnostics"`
}

func (r InspectionResult) Validate() error {
	if !validInspectionMode(r.Mode) {
		return fmt.Errorf("지원하지 않는 검사 방식입니다: %s", r.Mode)
	}
	if !validInspectionStatus(r.Status) {
		return fmt.Errorf("지원하지 않는 검사 상태입니다: %s", r.Status)
	}
	if !validRepairability(r.Repairability) {
		return fmt.Errorf("지원하지 않는 복구 가능성입니다: %s", r.Repairability)
	}
	if !validHealthStatus(r.Container.Status) ||
		!validHealthStatus(r.Container.IndexStatus) ||
		!validHealthStatus(r.Video.Status) ||
		!validHealthStatus(r.Audio.Status) ||
		!validHealthStatus(r.Timestamps.Status) ||
		!validHealthStatus(r.AVSync.Status) {
		return fmt.Errorf("검사 상세 상태에 지원하지 않는 값이 포함되어 있습니다")
	}
	if !validMoovAtomStatus(r.Container.MoovAtomStatus) {
		return fmt.Errorf("지원하지 않는 moov atom 상태입니다: %s", r.Container.MoovAtomStatus)
	}
	if !validRepairStrategy(r.Recommendation.Strategy) {
		return fmt.Errorf("지원하지 않는 복구 전략입니다: %s", r.Recommendation.Strategy)
	}
	if strings.TrimSpace(r.File.Path) == "" {
		return fmt.Errorf("검사 결과의 파일 경로가 필요합니다")
	}
	if r.DeepInspectionRecommended && strings.TrimSpace(r.DeepInspectionReason) == "" {
		return fmt.Errorf("정밀 검사 권장 사유가 필요합니다")
	}
	if r.File.SizeBytes < 0 || r.File.DurationSeconds < 0 {
		return fmt.Errorf("검사 결과의 파일 크기 또는 재생 시간이 올바르지 않습니다")
	}
	for index, damageRange := range r.DamageRanges {
		if damageRange.StartSeconds < 0 || damageRange.EndSeconds < damageRange.StartSeconds {
			return fmt.Errorf("손상 구간 %d의 시간 범위가 올바르지 않습니다", index+1)
		}
		if damageRange.ErrorCount < 0 {
			return fmt.Errorf("손상 구간 %d의 오류 수가 올바르지 않습니다", index+1)
		}
	}
	for _, position := range []*float64{r.FirstErrorSeconds, r.LastErrorSeconds, r.LastHealthySeconds} {
		if position != nil && *position < 0 {
			return fmt.Errorf("검사 위치 값은 0 이상이어야 합니다")
		}
	}
	return nil
}

func validInspectionMode(value InspectionMode) bool {
	return value == InspectionModeQuick || value == InspectionModeDeep
}

func validInspectionStatus(value InspectionStatus) bool {
	switch value {
	case InspectionStatusNormal, InspectionStatusWarning, InspectionStatusDamaged, InspectionStatusFailed:
		return true
	default:
		return false
	}
}

func validRepairability(value Repairability) bool {
	switch value {
	case RepairabilityNotNeeded, RepairabilityLossless, RepairabilityPartial, RepairabilityReencode, RepairabilityImpossible:
		return true
	default:
		return false
	}
}

func validHealthStatus(value HealthStatus) bool {
	switch value {
	case HealthStatusUnknown, HealthStatusNormal, HealthStatusWarning, HealthStatusDamaged, HealthStatusUnavailable:
		return true
	default:
		return false
	}
}

func validRepairStrategy(value RepairStrategy) bool {
	switch value {
	case RepairStrategyNone,
		RepairStrategyCompatibilityRemux,
		RepairStrategyRemux,
		RepairStrategyTimestampRemux,
		RepairStrategyPartial,
		RepairStrategyTruncate,
		RepairStrategyReencode,
		RepairStrategyUnavailable:
		return true
	default:
		return false
	}
}

func validMoovAtomStatus(value MoovAtomStatus) bool {
	switch value {
	case MoovAtomNotApplicable, MoovAtomPresent, MoovAtomMissing, MoovAtomUnknown:
		return true
	default:
		return false
	}
}
