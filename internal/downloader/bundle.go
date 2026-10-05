package downloader

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	managedToolsDirectoryName = "CHZZK Video Downloader"
	bundleManifestName         = "bundle-manifest.generated.json"
	bundleMarkerName           = ".bundle-id"
)

var (
	errBundledToolsUnavailable = errors.New("Windows 다운로드 도구 bundle이 준비되지 않았습니다")
	bundledToolsMaterializeMu   sync.Mutex
)

type bundledToolManifest struct {
	BundleID string            `json:"bundleId"`
	Platform string            `json:"platform"`
	Tools    []bundledToolFile `json:"tools"`
}

type bundledToolFile struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Source string `json:"source,omitempty"`
}

func managedToolsDir() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("사용자 LocalAppData 디렉터리를 확인할 수 없습니다: %w", err)
	}
	if strings.TrimSpace(cacheDir) == "" {
		return "", fmt.Errorf("사용자 LocalAppData 디렉터리를 확인할 수 없습니다")
	}
	return filepath.Join(cacheDir, managedToolsDirectoryName, "tools"), nil
}

func materializeBundledTools(bundle fs.FS, root, targetDir string) error {
	bundledToolsMaterializeMu.Lock()
	defer bundledToolsMaterializeMu.Unlock()

	manifestBytes, err := fs.ReadFile(bundle, filepath.ToSlash(filepath.Join(root, bundleManifestName)))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return errBundledToolsUnavailable
		}
		return fmt.Errorf("다운로드 도구 bundle manifest를 읽을 수 없습니다: %w", err)
	}

	var manifest bundledToolManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return fmt.Errorf("다운로드 도구 bundle manifest 형식이 올바르지 않습니다: %w", err)
	}
	manifest.BundleID = strings.TrimSpace(manifest.BundleID)
	if manifest.BundleID == "" || manifest.BundleID == "unprepared" || len(manifest.Tools) == 0 {
		return errBundledToolsUnavailable
	}

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("다운로드 도구 관리 디렉터리를 만들 수 없습니다: %w", err)
	}

	markerPath := filepath.Join(targetDir, bundleMarkerName)
	if currentID, readErr := os.ReadFile(markerPath); readErr == nil &&
		strings.TrimSpace(string(currentID)) == manifest.BundleID &&
		bundledToolFilesExist(targetDir, manifest.Tools) {
		return nil
	}

	for _, tool := range manifest.Tools {
		name := filepath.Base(strings.TrimSpace(tool.Name))
		if name == "." || name == "" || name != strings.TrimSpace(tool.Name) {
			return fmt.Errorf("다운로드 도구 bundle 파일명이 올바르지 않습니다: %q", tool.Name)
		}

		data, err := fs.ReadFile(bundle, filepath.ToSlash(filepath.Join(root, name)))
		if err != nil {
			return fmt.Errorf("%s bundle 파일을 읽을 수 없습니다: %w", name, err)
		}
		actualHash := sha256.Sum256(data)
		actualHashText := hex.EncodeToString(actualHash[:])
		expectedHash := strings.ToLower(strings.TrimSpace(tool.SHA256))
		if expectedHash == "" || actualHashText != expectedHash {
			return fmt.Errorf("%s bundle SHA-256 검증에 실패했습니다", name)
		}

		if err := writeBundledTool(filepath.Join(targetDir, name), data); err != nil {
			return err
		}
	}

	if err := os.WriteFile(markerPath, []byte(manifest.BundleID+"\n"), 0o644); err != nil {
		return fmt.Errorf("다운로드 도구 bundle 상태를 기록할 수 없습니다: %w", err)
	}
	return nil
}

func bundledToolFilesExist(targetDir string, tools []bundledToolFile) bool {
	for _, tool := range tools {
		info, err := os.Stat(filepath.Join(targetDir, filepath.Base(tool.Name)))
		if err != nil || info.IsDir() || info.Size() <= 0 {
			return false
		}
	}
	return true
}

func writeBundledTool(target string, data []byte) error {
	temp := target + ".tmp"
	if err := os.WriteFile(temp, data, 0o755); err != nil {
		return fmt.Errorf("%s 임시 파일을 만들 수 없습니다: %w", filepath.Base(target), err)
	}
	if err := os.Rename(temp, target); err != nil {
		if removeErr := os.Remove(target); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			_ = os.Remove(temp)
			return fmt.Errorf("%s 기존 bundle 파일을 제거할 수 없습니다: %w", filepath.Base(target), removeErr)
		}
		if retryErr := os.Rename(temp, target); retryErr != nil {
			_ = os.Remove(temp)
			return fmt.Errorf("%s bundle 파일을 교체할 수 없습니다: %w", filepath.Base(target), retryErr)
		}
	}
	return nil
}

func appendBundleError(status ToolchainStatus, bundleErr error) ToolchainStatus {
	if bundleErr == nil {
		return status
	}
	message := bundleErr.Error()
	for _, tool := range []*ToolStatus{&status.YTDLP, &status.FFmpeg, &status.FFprobe} {
		if tool.Available {
			continue
		}
		if tool.Error == "" {
			tool.Error = message
		} else {
			tool.Error += "; " + message
		}
	}
	return status
}
