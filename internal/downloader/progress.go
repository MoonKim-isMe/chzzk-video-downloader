package downloader

import (
	"strconv"
	"strings"
)

const (
	progressPrefix  = "__CHZZK_PROGRESS__"
	finalPathPrefix = "__CHZZK_FILE__"

	progressStatusFallbackPreparing   = "fallback_preparing"
	progressStatusFallbackDownloading = "fallback_downloading"
)

type DownloadProgress struct {
	Status              string  `json:"status"`
	Percent             float64 `json:"percent"`
	DownloadedBytes     int64   `json:"downloadedBytes"`
	TotalBytes          int64   `json:"totalBytes"`
	TotalBytesEstimated bool    `json:"totalBytesEstimated"`
	SpeedBytesPerSecond float64 `json:"speedBytesPerSecond"`
	ETASeconds          int64   `json:"etaSeconds"`
}

func parseProgressLine(line string) (DownloadProgress, bool) {
	if !strings.HasPrefix(line, progressPrefix) {
		return DownloadProgress{}, false
	}
	fields := strings.Split(strings.TrimPrefix(line, progressPrefix), "\t")
	if len(fields) != 7 {
		return DownloadProgress{}, false
	}

	downloaded := parseInt64(fields[1])
	total := parseInt64(fields[2])
	estimated := false
	if total <= 0 {
		total = parseInt64(fields[3])
		estimated = total > 0
	}

	return DownloadProgress{
		Status:              cleanTemplateValue(fields[0]),
		Percent:             parsePercent(fields[6]),
		DownloadedBytes:     downloaded,
		TotalBytes:          total,
		TotalBytesEstimated: estimated,
		SpeedBytesPerSecond: parseFloat64(fields[4]),
		ETASeconds:          parseInt64(fields[5]),
	}, true
}

func parseFinalPath(line string) (string, bool) {
	if !strings.HasPrefix(line, finalPathPrefix) {
		return "", false
	}
	path := strings.TrimSpace(strings.TrimPrefix(line, finalPathPrefix))
	return path, path != "" && path != "NA"
}

func cleanTemplateValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "NA" || value == "None" {
		return ""
	}
	return value
}

func parseInt64(value string) int64 {
	value = cleanTemplateValue(value)
	if value == "" {
		return 0
	}
	number, err := strconv.ParseFloat(value, 64)
	if err != nil || number < 0 {
		return 0
	}
	return int64(number)
}

func parseFloat64(value string) float64 {
	value = cleanTemplateValue(value)
	if value == "" {
		return 0
	}
	number, err := strconv.ParseFloat(value, 64)
	if err != nil || number < 0 {
		return 0
	}
	return number
}

func parsePercent(value string) float64 {
	value = strings.TrimSpace(strings.TrimSuffix(cleanTemplateValue(value), "%"))
	if value == "" {
		return 0
	}
	number, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	if number < 0 {
		return 0
	}
	if number > 100 {
		return 100
	}
	return number
}
