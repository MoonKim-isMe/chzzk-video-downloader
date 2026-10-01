package main

import (
	"path/filepath"
	"testing"

	appsettings "github.com/MoonKim-isMe/chzzk-video-downloader/internal/settings"
)

func TestGetSettingsReturnsDefaults(t *testing.T) {
	app := NewApp()
	store, err := appsettings.NewStore(appsettings.Defaults(filepath.Join(t.TempDir(), "downloads")))
	if err != nil {
		t.Fatal(err)
	}
	app.settingsStore = store

	value, err := app.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	if value.Resolution != appsettings.ResolutionBest ||
		value.OutputFormat != appsettings.OutputFormatMP4 ||
		value.MaxConcurrentDownloads != 1 {
		t.Fatalf("unexpected settings: %#v", value)
	}
}

func TestUpdateSettingsStoresNormalizedValue(t *testing.T) {
	app := NewApp()
	store, err := appsettings.NewStore(appsettings.Defaults(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	app.settingsStore = store

	next, err := app.UpdateSettings(appsettings.AppSettings{
		DownloadDir:            filepath.Join(t.TempDir(), "video"),
		Resolution:             appsettings.Resolution(" 1440P "),
		OutputFormat:           appsettings.OutputFormat(" WEBM "),
		MaxConcurrentDownloads: 3,
	})
	if err != nil {
		t.Fatal(err)
	}

	if next.Resolution != appsettings.Resolution1440p ||
		next.OutputFormat != appsettings.OutputFormatWebM ||
		next.MaxConcurrentDownloads != 3 {
		t.Fatalf("unexpected normalized settings: %#v", next)
	}

	current, err := app.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	if current != next {
		t.Fatalf("settings mismatch: current=%#v next=%#v", current, next)
	}
}

func TestUpdateSettingsRejectsInvalidValueWithoutChangingStore(t *testing.T) {
	app := NewApp()
	store, err := appsettings.NewStore(appsettings.Defaults(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	app.settingsStore = store
	before := store.Get()

	_, err = app.UpdateSettings(appsettings.AppSettings{
		DownloadDir:            t.TempDir(),
		Resolution:             "480p",
		OutputFormat:           appsettings.OutputFormatMP4,
		MaxConcurrentDownloads: 1,
	})
	if err == nil {
		t.Fatal("expected validation error")
	}

	after, err := app.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("invalid update changed settings: before=%#v after=%#v", before, after)
	}
}
