package downloader

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	temporaryDownloadDirectoryName       = ".chzzk-temp"
	nativeTemporaryDirectoryName         = "native"
	fallbackTemporaryDirectoryNamePrefix = "fallback-"
)

func TemporaryDownloadDir(outputDir string, videoNo int64) (string, error) {
	if videoNo <= 0 {
		return "", fmt.Errorf("유효한 VOD 번호가 필요합니다")
	}
	normalized, err := normalizeOutputDir(outputDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(normalized, temporaryDownloadDirectoryName, strconv.FormatInt(videoNo, 10)), nil
}

func CleanupTemporaryDownload(outputDir string, videoNo int64) error {
	tempDir, err := TemporaryDownloadDir(outputDir, videoNo)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(tempDir); err != nil {
		return fmt.Errorf("다운로드 임시 파일을 정리할 수 없습니다: %w", err)
	}
	parent := filepath.Dir(tempDir)
	if entries, readErr := os.ReadDir(parent); readErr == nil && len(entries) == 0 {
		_ = os.Remove(parent)
	}
	return nil
}

func temporaryDownloadDirForRequest(request DownloadRequest) (string, error) {
	videoURL, err := normalizeVideoURL(request.URL)
	if err != nil {
		return "", err
	}
	videoNo, err := videoNoFromNormalizedURL(videoURL)
	if err != nil {
		return "", err
	}
	return TemporaryDownloadDir(request.OutputDir, videoNo)
}

func nativeTemporaryDownloadDir(outputDir string, videoNo int64) (string, error) {
	root, err := TemporaryDownloadDir(outputDir, videoNo)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, nativeTemporaryDirectoryName), nil
}

func fallbackTemporaryDownloadDir(outputDir string, videoNo int64, runID string) (string, error) {
	root, err := TemporaryDownloadDir(outputDir, videoNo)
	if err != nil {
		return "", err
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return "", fmt.Errorf("대체 다운로드 실행 ID가 필요합니다")
	}
	return filepath.Join(root, fallbackTemporaryDirectoryNamePrefix+runID), nil
}

func nativeTemporaryDownloadDirForRequest(request DownloadRequest) (string, error) {
	videoURL, err := normalizeVideoURL(request.URL)
	if err != nil {
		return "", err
	}
	videoNo, err := videoNoFromNormalizedURL(videoURL)
	if err != nil {
		return "", err
	}
	return nativeTemporaryDownloadDir(request.OutputDir, videoNo)
}

func fallbackTemporaryDownloadDirForRequest(request DownloadRequest, runID string) (string, error) {
	videoURL, err := normalizeVideoURL(request.URL)
	if err != nil {
		return "", err
	}
	videoNo, err := videoNoFromNormalizedURL(videoURL)
	if err != nil {
		return "", err
	}
	return fallbackTemporaryDownloadDir(request.OutputDir, videoNo, runID)
}

func cleanupTemporaryDownloadRequest(request DownloadRequest) error {
	videoURL, err := normalizeVideoURL(request.URL)
	if err != nil {
		return err
	}
	videoNo, err := videoNoFromNormalizedURL(videoURL)
	if err != nil {
		return err
	}
	return CleanupTemporaryDownload(request.OutputDir, videoNo)
}

func videoNoFromNormalizedURL(videoURL string) (int64, error) {
	const prefix = "https://chzzk.naver.com/video/"
	value := strings.TrimSpace(videoURL)
	if !strings.HasPrefix(value, prefix) {
		return 0, fmt.Errorf("유효한 치지직 VOD URL이 아닙니다")
	}
	videoNo, err := strconv.ParseInt(strings.TrimPrefix(value, prefix), 10, 64)
	if err != nil || videoNo <= 0 {
		return 0, fmt.Errorf("유효한 VOD 번호가 필요합니다")
	}
	return videoNo, nil
}


func temporaryDownloadSize(path string) (int64, error) {
	var total int64
	err := filepath.WalkDir(path, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) {
				return nil
			}
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("다운로드 임시 파일 크기를 확인할 수 없습니다: %w", err)
	}
	return total, nil
}
