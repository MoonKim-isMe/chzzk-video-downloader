package downloader

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const toolsDirEnv = "CHZZK_DOWNLOADER_TOOLS_DIR"

type ToolStatus struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Version   string `json:"version"`
	Source    string `json:"source"`
	Found     bool   `json:"found"`
	Available bool   `json:"available"`
	Error     string `json:"error,omitempty"`
}

type ToolchainStatus struct {
	YTDLP         ToolStatus `json:"ytDlp"`
	FFmpeg        ToolStatus `json:"ffmpeg"`
	FFprobe       ToolStatus `json:"ffprobe"`
	DownloadReady bool       `json:"downloadReady"`
	MergeReady    bool       `json:"mergeReady"`
}

func (s ToolchainStatus) DownloadReadinessError() error {
	if s.DownloadReady {
		return nil
	}
	return fmt.Errorf(
		"영상 다운로드 실행 환경이 준비되지 않았습니다: %s",
		formatToolDiagnostic(s.YTDLP),
	)
}

func (s ToolchainStatus) MergeReadinessError() error {
	if s.MergeReady {
		return nil
	}

	failures := make([]string, 0, 2)
	if !s.FFmpeg.Available {
		failures = append(failures, formatToolDiagnostic(s.FFmpeg))
	}
	if !s.FFprobe.Available {
		failures = append(failures, formatToolDiagnostic(s.FFprobe))
	}
	if len(failures) == 0 {
		return fmt.Errorf("다운로드 후 영상 처리 환경이 준비되지 않았습니다")
	}
	return fmt.Errorf(
		"다운로드 후 영상 처리 환경이 준비되지 않았습니다: %s",
		strings.Join(failures, "; "),
	)
}

func formatToolDiagnostic(status ToolStatus) string {
	path := strings.TrimSpace(status.Path)
	if path == "" {
		path = "찾지 못함"
	}
	cause := strings.TrimSpace(status.Error)
	if cause == "" {
		cause = "사용 가능 여부를 확인할 수 없습니다"
	}
	name := strings.TrimSpace(status.Name)
	if name == "" {
		name = "도구"
	}
	return fmt.Sprintf(
		"%s [경로: %s, 사용 가능: 아니오, 오류: %s]",
		name,
		path,
		cause,
	)
}

type searchDir struct {
	path   string
	source string
}

type Resolver struct {
	searchDirs []searchDir
	lookPath   func(string) (string, error)
	probe      func(context.Context, string, string) (string, error)
}

func NewResolver() *Resolver {
	return &Resolver{
		searchDirs: defaultSearchDirs(),
		lookPath:   exec.LookPath,
		probe:      probeToolVersion,
	}
}

func defaultSearchDirs() []searchDir {
	var dirs []searchDir
	if explicit := strings.TrimSpace(os.Getenv(toolsDirEnv)); explicit != "" {
		dirs = append(dirs, searchDir{path: explicit, source: "environment"})
	}

	if managed, err := managedToolsDir(); err == nil {
		dirs = append(dirs, searchDir{path: managed, source: "managed-bundle"})
	}

	executable, err := os.Executable()
	if err == nil {
		if resolved, resolveErr := filepath.EvalSymlinks(executable); resolveErr == nil {
			executable = resolved
		}
		dir := filepath.Dir(executable)
		dirs = append(dirs,
			searchDir{path: filepath.Join(dir, "tools"), source: "app-tools"},
			searchDir{path: dir, source: "app"},
		)
	}
	return dedupeSearchDirs(dirs)
}

func dedupeSearchDirs(dirs []searchDir) []searchDir {
	seen := make(map[string]struct{}, len(dirs))
	result := make([]searchDir, 0, len(dirs))
	for _, dir := range dirs {
		path := filepath.Clean(strings.TrimSpace(dir.path))
		if path == "." || path == "" {
			continue
		}
		key := strings.ToLower(path)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		dir.path = path
		result = append(result, dir)
	}
	return result
}

func (r *Resolver) Resolve(ctx context.Context) ToolchainStatus {
	status := ToolchainStatus{
		YTDLP:   r.resolveTool(ctx, "yt-dlp", []string{"yt-dlp.exe", "yt-dlp"}, "--version"),
		FFmpeg:  r.resolveTool(ctx, "ffmpeg", []string{"ffmpeg.exe", "ffmpeg"}, "-version"),
		FFprobe: r.resolveTool(ctx, "ffprobe", []string{"ffprobe.exe", "ffprobe"}, "-version"),
	}
	status.DownloadReady = status.YTDLP.Available
	status.MergeReady = status.FFmpeg.Available && status.FFprobe.Available
	return status
}

func (r *Resolver) resolveTool(ctx context.Context, name string, filenames []string, versionArg string) ToolStatus {
	status := ToolStatus{Name: name}
	path, source, found := r.findBinary(filenames)
	if !found {
		status.Error = fmt.Sprintf("%s 실행 파일을 찾을 수 없습니다", name)
		return status
	}

	status.Path = path
	status.Source = source
	status.Found = true

	version, err := r.probe(ctx, path, versionArg)
	if err != nil {
		status.Error = err.Error()
		return status
	}
	status.Version = version
	status.Available = true
	return status
}

func (r *Resolver) findBinary(filenames []string) (string, string, bool) {
	for _, dir := range r.searchDirs {
		for _, filename := range filenames {
			candidate := filepath.Join(dir.path, filename)
			info, err := os.Stat(candidate)
			if err == nil && !info.IsDir() {
				absolute, absErr := filepath.Abs(candidate)
				if absErr == nil {
					candidate = absolute
				}
				return candidate, dir.source, true
			}
		}
	}

	for _, filename := range filenames {
		path, err := r.lookPath(filename)
		if err == nil {
			absolute, absErr := filepath.Abs(path)
			if absErr == nil {
				path = absolute
			}
			return path, "path", true
		}
	}
	return "", "", false
}

func probeToolVersion(ctx context.Context, path, arg string) (string, error) {
	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	cmd := exec.CommandContext(probeCtx, path, arg)
	hideConsoleWindow(cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if probeCtx.Err() != nil {
			return "", fmt.Errorf("%s 버전 확인 시간 초과: %w", filepath.Base(path), probeCtx.Err())
		}
		return "", fmt.Errorf("%s 버전을 확인할 수 없습니다: %w", filepath.Base(path), err)
	}

	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line, nil
		}
	}
	return "", fmt.Errorf("%s 버전 응답이 비어 있습니다", filepath.Base(path))
}
