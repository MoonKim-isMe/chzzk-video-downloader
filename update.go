package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/MoonKim-isMe/chzzk-video-downloader/internal/updater"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed wails.json
var updateWailsConfigJSON []byte

func currentProductVersion() (string, error) {
	var config struct {
		Info struct {
			ProductVersion string `json:"productVersion"`
		} `json:"info"`
	}
	if err := json.Unmarshal(updateWailsConfigJSON, &config); err != nil {
		return "", fmt.Errorf("앱 버전 정보를 읽을 수 없습니다: %w", err)
	}
	version := strings.TrimSpace(config.Info.ProductVersion)
	if version == "" {
		return "", fmt.Errorf("앱 버전 정보가 비어 있습니다")
	}
	return version, nil
}

func (a *App) CheckForUpdates() (updater.Result, error) {
	version, err := currentProductVersion()
	if err != nil {
		return updater.Result{}, err
	}
	return updater.Check(a.appContext(), version)
}

func (a *App) OpenReleasePage(rawURL string) error {
	releaseURL, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return fmt.Errorf("Release 페이지 주소가 올바르지 않습니다: %w", err)
	}
	if releaseURL.Scheme != "https" ||
		!strings.EqualFold(releaseURL.Host, "github.com") ||
		!strings.HasPrefix(releaseURL.Path, "/MoonKim-isMe/chzzk-video-downloader/releases/") {
		return fmt.Errorf("허용되지 않은 Release 페이지 주소입니다")
	}
	runtime.BrowserOpenURL(a.appContext(), releaseURL.String())
	return nil
}
