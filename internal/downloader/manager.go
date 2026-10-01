package downloader

import (
	"context"
	"fmt"
	"os"
)

type Manager struct {
	resolver *Resolver
	runner   *Runner
}

func NewManager() *Manager {
	return &Manager{
		resolver: NewResolver(),
		runner:   NewRunner(),
	}
}

func (m *Manager) ToolchainStatus(ctx context.Context) ToolchainStatus {
	return m.resolver.Resolve(ctx)
}

func (m *Manager) Prepare(ctx context.Context, request DownloadRequest) (CommandSpec, ToolchainStatus, error) {
	toolchain := m.ToolchainStatus(ctx)
	if !toolchain.DownloadReady {
		return CommandSpec{}, toolchain, fmt.Errorf("yt-dlp 실행 환경이 준비되지 않았습니다")
	}
	if !toolchain.MergeReady {
		return CommandSpec{}, toolchain, fmt.Errorf("ffmpeg/ffprobe 실행 환경이 준비되지 않았습니다")
	}
	command, err := BuildDownloadCommand(toolchain, request)
	if err != nil {
		return CommandSpec{}, toolchain, err
	}
	outputDir, err := normalizeOutputDir(request.OutputDir)
	if err != nil {
		return CommandSpec{}, toolchain, err
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return CommandSpec{}, toolchain, fmt.Errorf("다운로드 폴더를 만들 수 없습니다: %w", err)
	}
	return command, toolchain, nil
}

func (m *Manager) Runner() *Runner { return m.runner }
