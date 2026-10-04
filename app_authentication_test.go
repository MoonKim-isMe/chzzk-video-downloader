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

	cookiesFile := filepath.Join(t.TempDir(), "cookies.txt")
	updated, err := app.UpdateAuthenticationSettings(appsettings.AuthenticationSettings{
		Enabled: true, CookiesFilePath: " " + cookiesFile + " ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Enabled || updated.CookiesFilePath != cookiesFile {
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

func TestActiveAuthenticationCookiesFileDisabled(t *testing.T) {
	app := NewApp()

	got, err := app.activeAuthenticationCookiesFile()
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("expected empty cookies path, got %q", got)
	}
}

func TestActiveAuthenticationCookiesFileEnabled(t *testing.T) {
	app := NewApp()
	cookiesFile := filepath.Join(t.TempDir(), "cookies.txt")
	if _, err := app.UpdateAuthenticationSettings(appsettings.AuthenticationSettings{
		Enabled:         true,
		CookiesFilePath: cookiesFile,
	}); err != nil {
		t.Fatal(err)
	}

	got, err := app.activeAuthenticationCookiesFile()
	if err != nil {
		t.Fatal(err)
	}
	if got != cookiesFile {
		t.Fatalf("unexpected cookies path: got %q want %q", got, cookiesFile)
	}
}
