package main

import (
	"embed"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		println("Error:", err.Error())
		return
	}

	err = wails.Run(&options.App{
		Title:            appName,
		Width:            1280,
		Height:           800,
		MinWidth:         960,
		MinHeight:        640,
		AssetServer:      &assetserver.Options{Assets: assets},
		BackgroundColour: &options.RGBA{R: 15, G: 23, B: 42, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind:             []interface{}{app},
		Windows: &windows.Options{
			WebviewUserDataPath: filepath.Join(cacheDir, appName, "webview2"),
		},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}
