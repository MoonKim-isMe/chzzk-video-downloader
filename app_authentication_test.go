package main

import (
	"context"
	"path/filepath"
	"testing"

	appsettings "github.com/MoonKim-isMe/chzzk-video-downloader/internal/settings"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func TestAuthenticationSettingsDefaultsAndUpdate(t *testing.T) {
	app := NewApp()
	current, err := app.GetAuthenticationSettings()
	if err != nil {
		t.Fatal(err)
	}
	if current != appsettings.AuthenticationDefaults() {
		t.Fatalf("unexpected authentication defaults: %#v", current)
	}
	updated, err := app.UpdateAuthenticationSettings(appsettings.AuthenticationSettings{
		Enabled: true, Mode: appsettings.AuthenticationMode(" BROWSER "),
		Browser: appsettings.AuthenticationBrowser(" WHALE "), BrowserProfile: " Profile 1 ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Enabled || updated.Mode != appsettings.AuthenticationModeBrowser ||
		updated.Browser != appsettings.AuthenticationBrowserWhale || updated.BrowserProfile != "Profile 1" {
		t.Fatalf("unexpected authentication update: %#v", updated)
	}
}

func TestSelectAuthenticationCookiesFileUsesCurrentFileDirectory(t *testing.T) {
	app := NewApp()
	current := filepath.Join(t.TempDir(), "auth", "cookies.txt")
	expected := filepath.Join(t.TempDir(), "selected", "cookies.txt")
	var gotDefault string
	app.filePicker = func(ctx context.Context, options runtime.OpenDialogOptions) (string, error) {
		gotDefault = options.DefaultDirectory
		if options.Title != "쿠키 파일 선택" {
			t.Fatalf("unexpected title: %s", options.Title)
		}
		return expected, nil
	}
	selected, err := app.SelectAuthenticationCookiesFile(current)
	if err != nil {
		t.Fatal(err)
	}
	if gotDefault != filepath.Dir(current) {
		t.Fatalf("unexpected default directory: got %s want %s", gotDefault, filepath.Dir(current))
	}
	if selected != expected {
		t.Fatalf("unexpected selected path: got %s want %s", selected, expected)
	}
}
