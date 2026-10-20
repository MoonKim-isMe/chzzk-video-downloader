package downloader

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// The FFmpeg distributed with the app is not guaranteed to include libx264.
// Prefer H.264 when present; MPEG-4 Part 2 is a software-only fallback supported
// by minimal FFmpeg builds and remains playable in MP4 and MKV containers.
func probeSegmentVideoEncoder(ctx context.Context, ffmpegPath, outputFormat string) (string, error) {
	if strings.TrimSpace(ffmpegPath) == "" {
		return "", fmt.Errorf("FFmpeg 실행 파일 경로를 확인할 수 없습니다")
	}
	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	command := exec.CommandContext(probeCtx, ffmpegPath, "-hide_banner", "-encoders")
	hideConsoleWindow(command)
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("FFmpeg 영상 인코더 목록을 확인하지 못했습니다: %w", err)
	}
	return chooseSegmentVideoEncoder(outputFormat, string(output))
}

func chooseSegmentVideoEncoder(outputFormat, listing string) (string, error) {
	encoders := make(map[string]bool)
	for _, line := range strings.Split(listing, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || len(fields[0]) != 6 || fields[0][0] != 'V' {
			continue
		}
		encoders[fields[1]] = true
	}
	if outputFormat == "webm" {
		switch {
		case encoders["libvpx-vp9"]:
			return "libvpx-vp9", nil
		case encoders["libvpx"]:
			return "libvpx", nil
		default:
			return "", fmt.Errorf("현재 FFmpeg에는 WebM 구간 변환용 VP8/VP9 인코더가 없습니다")
		}
	}
	switch {
	case encoders["libx264"]:
		return "libx264", nil
	case encoders["mpeg4"]:
		return "mpeg4", nil
	default:
		return "", fmt.Errorf("현재 FFmpeg에는 구간 영상 변환용 libx264 또는 mpeg4 인코더가 없습니다")
	}
}

// Return output codec options shared by the signed HLS fallback and the
// full-VOD-discontinuity recovery path, so neither silently assumes libx264.
func segmentVideoEncodingArgs(outputFormat, encoder string) ([]string, error) {
	if outputFormat == "webm" {
		if encoder != "libvpx-vp9" && encoder != "libvpx" {
			return nil, fmt.Errorf("WebM 영상 인코더가 올바르지 않습니다: %s", encoder)
		}
		return []string{"-c:v", encoder, "-deadline", "realtime", "-cpu-used", "5", "-c:a", "libopus"}, nil
	}
	if outputFormat != "mp4" && outputFormat != "mkv" {
		return nil, fmt.Errorf("지원하지 않는 영상 구간 포맷입니다: %s", outputFormat)
	}
	switch encoder {
	case "libx264":
		return []string{"-c:v", "libx264", "-preset", "fast", "-crf", "20", "-pix_fmt", "yuv420p", "-c:a", "aac", "-b:a", "192k"}, nil
	case "mpeg4":
		// Native mpeg4 does not accept x264-only -preset/-crf options.
		return []string{"-c:v", "mpeg4", "-q:v", "3", "-pix_fmt", "yuv420p", "-c:a", "aac", "-b:a", "192k"}, nil
	default:
		return nil, fmt.Errorf("지원하지 않는 구간 영상 인코더입니다: %s", encoder)
	}
}
