package downloader

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestProbeToolVersionReadsFirstLine(t *testing.T) {
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")
	version, err := probeToolVersion(context.Background(), os.Args[0], "version")
	if err != nil || version != "1.2.3" {
		t.Fatalf("unexpected tool version: %q, error: %v", version, err)
	}
}

func TestProbeToolVersionReportsProcessFailure(t *testing.T) {
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")
	if _, err := probeToolVersion(context.Background(), os.Args[0], "fail"); err == nil {
		t.Fatal("failed version probe must return an error")
	}
}

func TestProbeToolVersionRespectsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := probeToolVersion(ctx, os.Args[0], "version"); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled probe, got %v", err)
	}
}

func TestResolverPrefersConfiguredDirectory(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"yt-dlp.exe", "ffmpeg.exe", "ffprobe.exe"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("test"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	resolver := &Resolver{
		searchDirs: []searchDir{{path: dir, source: "test"}},
		lookPath: func(string) (string, error) {
			return "", errors.New("not found")
		},
		probe: func(_ context.Context, path, arg string) (string, error) {
			return filepath.Base(path) + " " + arg, nil
		},
	}

	status := resolver.Resolve(context.Background())
	if !status.DownloadReady || !status.MergeReady {
		t.Fatalf("unexpected status: %#v", status)
	}
	if status.YTDLP.Source != "test" || filepath.Base(status.YTDLP.Path) != "yt-dlp.exe" {
		t.Fatalf("unexpected yt-dlp resolution: %#v", status.YTDLP)
	}
}

func TestResolverReportsUnusableBinary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "yt-dlp")
	if err := os.WriteFile(path, []byte("test"), 0o755); err != nil {
		t.Fatal(err)
	}
	resolver := &Resolver{
		searchDirs: []searchDir{{path: dir, source: "test"}},
		lookPath:   func(string) (string, error) { return "", errors.New("not found") },
		probe: func(_ context.Context, path, arg string) (string, error) {
			return "", errors.New("broken")
		},
	}
	status := resolver.Resolve(context.Background())
	if !status.YTDLP.Found || status.YTDLP.Available || status.DownloadReady {
		t.Fatalf("unexpected status: %#v", status.YTDLP)
	}
}

func TestResolverPrefersManagedBundleBeforeAppAndPath(t *testing.T) {
	managed := t.TempDir()
	appDir := t.TempDir()

	managedYTDLP := filepath.Join(managed, "yt-dlp.exe")
	if err := os.WriteFile(managedYTDLP, []byte("managed"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "yt-dlp.exe"), []byte("app"), 0o755); err != nil {
		t.Fatal(err)
	}

	resolver := &Resolver{
		searchDirs: []searchDir{
			{path: managed, source: "managed-bundle"},
			{path: appDir, source: "app"},
		},
		lookPath: func(string) (string, error) {
			return filepath.Join(t.TempDir(), "path-yt-dlp.exe"), nil
		},
		probe: func(_ context.Context, path, arg string) (string, error) {
			if filepath.Base(path) == "yt-dlp.exe" {
				return "test", nil
			}
			return "", errors.New("not found")
		},
	}

	status := resolver.Resolve(context.Background())
	if !status.YTDLP.Available ||
		status.YTDLP.Source != "managed-bundle" ||
		status.YTDLP.Path != managedYTDLP {
		t.Fatalf("unexpected managed resolution: %#v", status.YTDLP)
	}
}

func TestAppendBundleErrorOnlyTouchesUnavailableTools(t *testing.T) {
	status := ToolchainStatus{
		YTDLP:   ToolStatus{Name: "yt-dlp", Available: false, Error: "not found"},
		FFmpeg:  ToolStatus{Name: "ffmpeg", Available: true},
		FFprobe: ToolStatus{Name: "ffprobe", Available: false},
	}
	result := appendBundleError(status, errors.New("bundle missing"))

	if result.YTDLP.Error != "not found; bundle missing" {
		t.Fatalf("unexpected yt-dlp error: %q", result.YTDLP.Error)
	}
	if result.FFmpeg.Error != "" {
		t.Fatalf("available ffmpeg must not receive bundle error: %q", result.FFmpeg.Error)
	}
	if result.FFprobe.Error != "bundle missing" {
		t.Fatalf("unexpected ffprobe error: %q", result.FFprobe.Error)
	}
}


func TestDownloadReadinessErrorIncludesToolDiagnostics(t *testing.T) {
	status := ToolchainStatus{
		YTDLP: ToolStatus{
			Name:      "yt-dlp",
			Path:      `C:\Users\tester\AppData\Local\CHZZK Video Downloader\tools\yt-dlp.exe`,
			Found:     true,
			Available: false,
			Error:     "yt-dlp.exe 버전 확인 시간 초과: context deadline exceeded",
		},
		DownloadReady: false,
	}

	err := status.DownloadReadinessError()
	if err == nil {
		t.Fatal("unavailable yt-dlp must return a readiness error")
	}
	expected := `영상 다운로드 실행 환경이 준비되지 않았습니다: yt-dlp [사용 가능: 아니오, 오류: yt-dlp.exe 버전 확인 시간 초과: context deadline exceeded]`
	if err.Error() != expected {
		t.Fatalf("unexpected readiness error:\nwant: %s\n got: %s", expected, err)
	}
}

func TestMergeReadinessErrorIncludesOnlyUnavailableTools(t *testing.T) {
	status := ToolchainStatus{
		FFmpeg: ToolStatus{
			Name:      "ffmpeg",
			Path:      `C:\tools\ffmpeg.exe`,
			Found:     true,
			Available: true,
		},
		FFprobe: ToolStatus{
			Name:      "ffprobe",
			Found:     false,
			Available: false,
			Error:     "ffprobe 실행 파일을 찾을 수 없습니다",
		},
		MergeReady: false,
	}

	err := status.MergeReadinessError()
	if err == nil {
		t.Fatal("unavailable ffprobe must return a readiness error")
	}
	expected := `다운로드 후 영상 처리 환경이 준비되지 않았습니다: ffprobe [사용 가능: 아니오, 오류: ffprobe 실행 파일을 찾을 수 없습니다]`
	if err.Error() != expected {
		t.Fatalf("unexpected merge readiness error:\nwant: %s\n got: %s", expected, err)
	}
}
