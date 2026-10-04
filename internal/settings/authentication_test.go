package settings

import (
	"path/filepath"
	"testing"
)

func TestAuthenticationDefaults(t *testing.T) {
	value := AuthenticationDefaults()
	if value.Enabled || value.Mode != AuthenticationModeBrowser || value.Browser != AuthenticationBrowserChrome {
		t.Fatalf("unexpected authentication defaults: %#v", value)
	}
	if err := ValidateAuthentication(value); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizeAuthentication(t *testing.T) {
	value := NormalizeAuthentication(AuthenticationSettings{
		Enabled: true, Mode: AuthenticationMode(" BROWSER "), Browser: AuthenticationBrowser(" WHALE "),
		BrowserProfile: " Profile 1 ", CookiesFilePath: " ",
	})
	if value.Mode != AuthenticationModeBrowser || value.Browser != AuthenticationBrowserWhale ||
		value.BrowserProfile != "Profile 1" || value.CookiesFilePath != "" {
		t.Fatalf("unexpected normalized authentication settings: %#v", value)
	}
}

func TestValidateAuthenticationRequiresCookiesFileWhenEnabled(t *testing.T) {
	value := AuthenticationDefaults()
	value.Enabled = true
	value.Mode = AuthenticationModeCookiesFile
	if err := ValidateAuthentication(value); err == nil {
		t.Fatal("expected cookies file validation error")
	}
	value.CookiesFilePath = filepath.Join(t.TempDir(), "cookies.txt")
	if err := ValidateAuthentication(value); err != nil {
		t.Fatal(err)
	}
}

func TestAuthenticationStoreUpdate(t *testing.T) {
	store, err := NewAuthenticationStore(AuthenticationDefaults())
	if err != nil {
		t.Fatal(err)
	}
	updated, err := store.Update(AuthenticationSettings{
		Enabled: true, Mode: AuthenticationModeBrowser, Browser: AuthenticationBrowserEdge, BrowserProfile: "Default",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Enabled || updated.Browser != AuthenticationBrowserEdge || store.Get() != updated {
		t.Fatalf("unexpected authentication store value: %#v", updated)
	}
}
