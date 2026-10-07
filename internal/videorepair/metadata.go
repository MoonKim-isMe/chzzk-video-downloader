package videorepair

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const metadataProbeTimeout = 20 * time.Second

var supportedVideoExtensions = []string{
	".mp4",
	".m4v",
	".mov",
	".mkv",
	".webm",
	".avi",
	".ts",
	".m2ts",
	".mts",
}

type FileInfo struct {
	Path              string  `json:"path"`
	Name              string  `json:"name"`
	SizeBytes         int64   `json:"sizeBytes"`
	ModifiedUnixMilli  int64   `json:"modifiedUnixMilli"`
	Extension         string  `json:"extension"`
	Container         string  `json:"container"`
	ContainerSource   string  `json:"containerSource"`
	FormatName        string  `json:"formatName,omitempty"`
	DurationSeconds   float64 `json:"durationSeconds"`
	MetadataAvailable bool    `json:"metadataAvailable"`
	MetadataError     string  `json:"metadataError,omitempty"`
}

type ffprobeResponse struct {
	Format struct {
		FormatName string `json:"format_name"`
		Duration   string `json:"duration"`
	} `json:"format"`
}

func SupportedVideoExtensions() []string {
	return append([]string(nil), supportedVideoExtensions...)
}

func FileDialogPattern() string {
	patterns := make([]string, 0, len(supportedVideoExtensions))
	for _, extension := range supportedVideoExtensions {
		patterns = append(patterns, "*"+extension)
	}
	return strings.Join(patterns, ";")
}

func IsSupportedPath(path string) bool {
	extension := strings.ToLower(filepath.Ext(strings.TrimSpace(path)))
	for _, supported := range supportedVideoExtensions {
		if extension == supported {
			return true
		}
	}
	return false
}

func ProbeFile(ctx context.Context, ffprobePath, path string) (FileInfo, error) {
	info, err := basicFileInfo(path)
	if err != nil {
		return FileInfo{}, err
	}

	ffprobePath = strings.TrimSpace(ffprobePath)
	if ffprobePath == "" {
		info.MetadataError = "영상 기본 정보를 확인할 수 있는 도구가 준비되지 않았습니다."
		return info, nil
	}

	probeCtx, cancel := context.WithTimeout(ctx, metadataProbeTimeout)
	defer cancel()

	cmd := exec.CommandContext(
		probeCtx,
		ffprobePath,
		"-v", "error",
		"-show_entries", "format=format_name,duration",
		"-of", "json",
		info.Path,
	)
	hideConsoleWindow(cmd)
	output, runErr := cmd.Output()
	if runErr != nil {
		if probeCtx.Err() != nil {
			info.MetadataError = "영상 기본 정보 확인 시간이 초과되었습니다."
		} else {
			info.MetadataError = "영상의 컨테이너와 재생 시간을 확인하지 못했습니다."
		}
		return info, nil
	}

	formatName, duration, parseErr := parseProbeOutput(output)
	if parseErr != nil {
		info.MetadataError = "영상 기본 정보 응답을 해석하지 못했습니다."
		return info, nil
	}

	info.FormatName = formatName
	info.DurationSeconds = duration
	if normalized := normalizeContainer(formatName, info.Extension); normalized != "" {
		info.Container = normalized
		info.ContainerSource = "probe"
	}
	info.MetadataAvailable = true
	return info, nil
}

func basicFileInfo(path string) (FileInfo, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return FileInfo{}, fmt.Errorf("동영상 파일 경로가 필요합니다")
	}
	path = filepath.Clean(path)
	if !IsSupportedPath(path) {
		return FileInfo{}, fmt.Errorf("지원하지 않는 동영상 형식입니다: %s", filepath.Ext(path))
	}

	stat, err := os.Stat(path)
	if err != nil {
		return FileInfo{}, fmt.Errorf("동영상 파일을 확인할 수 없습니다: %w", err)
	}
	if stat.IsDir() {
		return FileInfo{}, fmt.Errorf("선택한 경로가 동영상 파일이 아닙니다")
	}

	extension := strings.ToLower(filepath.Ext(path))
	return FileInfo{
		Path:             path,
		Name:             filepath.Base(path),
		SizeBytes:        stat.Size(),
		ModifiedUnixMilli: stat.ModTime().UnixMilli(),
		Extension:        extension,
		Container:       containerFromExtension(extension),
		ContainerSource: "extension",
	}, nil
}

func parseProbeOutput(output []byte) (string, float64, error) {
	var payload ffprobeResponse
	if err := json.Unmarshal(output, &payload); err != nil {
		return "", 0, err
	}

	formatName := strings.TrimSpace(payload.Format.FormatName)
	duration := 0.0
	if value := strings.TrimSpace(payload.Format.Duration); value != "" && !strings.EqualFold(value, "N/A") {
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return "", 0, err
		}
		if !math.IsNaN(parsed) && !math.IsInf(parsed, 0) && parsed > 0 {
			duration = parsed
		}
	}
	return formatName, duration, nil
}

func normalizeContainer(formatName, extension string) string {
	formatName = strings.ToLower(strings.TrimSpace(formatName))
	extension = strings.ToLower(strings.TrimSpace(extension))

	switch {
	case strings.Contains(formatName, "matroska") || strings.Contains(formatName, "webm"):
		if extension == ".webm" {
			return "WebM"
		}
		return "Matroska"
	case strings.Contains(formatName, "mov") || strings.Contains(formatName, "mp4"):
		if extension == ".mov" {
			return "MOV"
		}
		return "MP4"
	case strings.Contains(formatName, "mpegts"):
		return "MPEG-TS"
	case strings.Contains(formatName, "avi"):
		return "AVI"
	}

	if formatName != "" {
		parts := strings.Split(formatName, ",")
		if len(parts) > 0 && strings.TrimSpace(parts[0]) != "" {
			return strings.ToUpper(strings.TrimSpace(parts[0]))
		}
	}
	return containerFromExtension(extension)
}

func containerFromExtension(extension string) string {
	switch strings.ToLower(strings.TrimSpace(extension)) {
	case ".mp4", ".m4v":
		return "MP4"
	case ".mov":
		return "MOV"
	case ".mkv":
		return "Matroska"
	case ".webm":
		return "WebM"
	case ".avi":
		return "AVI"
	case ".ts", ".m2ts", ".mts":
		return "MPEG-TS"
	default:
		return "확인 불가"
	}
}
