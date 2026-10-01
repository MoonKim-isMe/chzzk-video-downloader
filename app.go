package main

import (
	"context"

	"github.com/MoonKim-isMe/chzzk-video-downloader/internal/chzzk"
	"github.com/MoonKim-isMe/chzzk-video-downloader/internal/downloader"
)

const (
	appName    = "CHZZK Video Downloader"
	appVersion = "0.1.0"
)

type App struct {
	ctx             context.Context
	chzzkClient     *chzzk.Client
	channelStore    *chzzk.Store
	downloadManager *downloader.Manager
}

type AppInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Runtime string `json:"runtime"`
}

func NewApp() *App {
	return &App{
		chzzkClient:     chzzk.NewClient(),
		channelStore:    chzzk.NewStore(),
		downloadManager: downloader.NewManager(),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) appContext() context.Context {
	if a.ctx != nil {
		return a.ctx
	}
	return context.Background()
}

func (a *App) GetAppInfo() AppInfo {
	return AppInfo{
		Name:    appName,
		Version: appVersion,
		Runtime: "Wails v2",
	}
}

func (a *App) SearchChannels(keyword string, offset, size int) (chzzk.ChannelSearchResult, error) {
	return a.chzzkClient.SearchChannels(a.appContext(), keyword, offset, size)
}

func (a *App) ResolveChannelURL(rawURL string) (chzzk.Channel, error) {
	channelID, _, err := chzzk.ParseChannelURL(rawURL)
	if err != nil {
		return chzzk.Channel{}, err
	}
	return a.chzzkClient.GetChannel(a.appContext(), channelID)
}

func (a *App) GetSavedChannels() []chzzk.Channel {
	return a.channelStore.List()
}

func (a *App) SaveChannel(channel chzzk.Channel) ([]chzzk.Channel, error) {
	return a.channelStore.Save(channel)
}

func (a *App) RemoveSavedChannel(channelID string) []chzzk.Channel {
	return a.channelStore.Remove(channelID)
}

func (a *App) GetChannelVideos(channelID string, page, size int) (chzzk.VideoListResult, error) {
	return a.chzzkClient.GetChannelVideos(a.appContext(), channelID, page, size)
}

func (a *App) GetDownloadToolchainStatus() downloader.ToolchainStatus {
	return a.downloadManager.ToolchainStatus(a.appContext())
}
