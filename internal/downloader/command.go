package downloader

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	DefaultFormatSelector      = "bv*+ba/b"
	DefaultOutputTemplate      = "%(title)s [%(id)s].%(ext)s"
	DefaultConcurrentFragments = 2
	progressTemplate           = progressPrefix + "%(progress.status)s\t%(progress.downloaded_bytes)s\t%(progress.total_bytes)s\t%(progress.total_bytes_estimate)s\t%(progress.speed)s\t%(progress.eta)s\t%(progress._percent_str)s"
)

var videoPathPattern = regexp.MustCompile(`^/video/[0-9]+/?$`)

type DownloadRequest struct {
	URL                 string `json:"url"`
	OutputDir           string `json:"outputDir"`
	FormatSelector      string `json:"formatSelector,omitempty"`
	OutputTemplate      string `json:"outputTemplate,omitempty"`
	OutputFormat        string `json:"outputFormat,omitempty"`
	ConcurrentFragments int    `json:"concurrentFragments,omitempty"`
}

type CommandSpec struct {
	Path string   `json:"path"`
	Args []string `json:"args"`
	Env  []string `json:"-"`
}

func BuildDownloadCommand(toolchain ToolchainStatus, request DownloadRequest) (CommandSpec, error) {
	return buildDownloadCommand(toolchain, request, false)
}

func buildDownloadCommand(toolchain ToolchainStatus, request DownloadRequest, useFFmpegHLS bool) (CommandSpec, error) {
	tempDir, err := nativeTemporaryDownloadDirForRequest(request)
	if useFFmpegHLS {
		tempDir, err = fallbackTemporaryDownloadDirForRequest(request, "default")
	}
	if err != nil {
		return CommandSpec{}, err
	}
	return buildDownloadCommandWithTempDir(toolchain, request, useFFmpegHLS, tempDir)
}

func buildDownloadCommandWithTempDir(
	toolchain ToolchainStatus,
	request DownloadRequest,
	useFFmpegHLS bool,
	tempDir string,
) (CommandSpec, error) {
	if !toolchain.YTDLP.Available {
		return CommandSpec{}, fmt.Errorf("영상 다운로드 기능을 사용할 수 없습니다")
	}
	if !toolchain.MergeReady {
		return CommandSpec{}, fmt.Errorf("다운로드 후 영상 처리 기능을 사용할 수 없습니다")
	}

	videoURL, err := normalizeVideoURL(request.URL)
	if err != nil {
		return CommandSpec{}, err
	}

	outputDir, err := normalizeOutputDir(request.OutputDir)
	if err != nil {
		return CommandSpec{}, err
	}
	tempDir = filepath.Clean(strings.TrimSpace(tempDir))
	if tempDir == "" || tempDir == "." {
		return CommandSpec{}, fmt.Errorf("다운로드 임시 경로가 필요합니다")
	}

	formatSelector := strings.TrimSpace(request.FormatSelector)
	if formatSelector == "" {
		formatSelector = DefaultFormatSelector
	}
	outputTemplate := strings.TrimSpace(request.OutputTemplate)
	if outputTemplate == "" {
		outputTemplate = DefaultOutputTemplate
	}

	outputFormat, err := normalizeOutputFormat(request.OutputFormat)
	if err != nil {
		return CommandSpec{}, err
	}
	concurrentFragments, err := normalizeConcurrentFragments(request.ConcurrentFragments)
	if err != nil {
		return CommandSpec{}, err
	}

	var env []string
	args := []string{
		"--ignore-config",
		"--no-simulate",
		"--progress",
		"--newline",
		"--color", "never",
		"--no-playlist",
		"--windows-filenames",
		"--no-overwrites",
		"--continue",
		"--no-keep-fragments",
		"--concurrent-fragments", strconv.Itoa(concurrentFragments),
		"--progress-delta", "0.5",
		"--progress-template", "download:" + progressTemplate,
		"--print", "after_move:" + finalPathPrefix + "%(filepath)s",
		"--format", formatSelector,
		"--paths", outputDir,
		"--paths", "temp:" + tempDir,
		"--output", outputTemplate,
	}
	if useFFmpegHLS {
		args = append(
			args,
			"--downloader", "m3u8:ffmpeg",
			"--downloader-args", "ffmpeg:-nostdin -stats_period 1",
		)

		ffmpegDir := filepath.Dir(toolchain.FFmpeg.Path)
		fallbackPath := ffmpegDir
		if currentPath := strings.TrimSpace(os.Getenv("PATH")); currentPath != "" {
			fallbackPath += string(os.PathListSeparator) + currentPath
		}
		env = append(env, "PATH="+fallbackPath)
	}
	if outputFormat != "" {
		args = append(args,
			"--merge-output-format", outputFormat,
			"--remux-video", outputFormat,
		)
	}

	location, err := ffmpegLocation(toolchain)
	if err != nil {
		return CommandSpec{}, err
	}
	if location != "" {
		args = append(args, "--ffmpeg-location", location)
	}
	args = append(args, videoURL)

	return CommandSpec{Path: toolchain.YTDLP.Path, Args: args, Env: env}, nil
}

func normalizeVideoURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("VOD URL이 필요합니다")
	}
	parsed, err := url.Parse(value)
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") {
		return "", fmt.Errorf("유효한 치지직 VOD URL이 아닙니다")
	}
	if !strings.EqualFold(parsed.Hostname(), "chzzk.naver.com") || !videoPathPattern.MatchString(parsed.EscapedPath()) {
		return "", fmt.Errorf("치지직 VOD URL만 다운로드할 수 있습니다")
	}

	parsed.Scheme = "https"
	parsed.Host = "chzzk.naver.com"
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.Path = strings.TrimSuffix(parsed.Path, "/")
	parsed.RawPath = ""
	return parsed.String(), nil
}

func normalizeOutputFormat(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return "", nil
	}
	switch value {
	case "mp4", "mkv", "webm":
		return value, nil
	default:
		return "", fmt.Errorf("지원하지 않는 출력 포맷입니다: %s", value)
	}
}

func normalizeConcurrentFragments(value int) (int, error) {
	if value == 0 {
		return DefaultConcurrentFragments, nil
	}
	switch value {
	case 1, 2, 4, 8:
		return value, nil
	default:
		return 0, fmt.Errorf("지원하지 않는 다운로드 가속 값입니다: %d", value)
	}
}

func normalizeOutputDir(raw string) (string, error) {
	outputDir := strings.TrimSpace(raw)
	if outputDir == "" {
		return "", fmt.Errorf("다운로드 경로가 필요합니다")
	}
	return filepath.Clean(outputDir), nil
}

func ffmpegLocation(toolchain ToolchainStatus) (string, error) {
	if !toolchain.FFmpeg.Available || !toolchain.FFprobe.Available {
		return "", fmt.Errorf("다운로드 후 영상 처리 기능을 사용할 수 없습니다")
	}
	ffmpegDir := filepath.Dir(toolchain.FFmpeg.Path)
	ffprobeDir := filepath.Dir(toolchain.FFprobe.Path)
	if strings.EqualFold(filepath.Clean(ffmpegDir), filepath.Clean(ffprobeDir)) {
		return ffmpegDir, nil
	}
	if toolchain.FFmpeg.Source == "path" && toolchain.FFprobe.Source == "path" {
		return "", nil
	}
	return "", fmt.Errorf("다운로드 후 영상 처리 환경을 확인해 주세요")
}
