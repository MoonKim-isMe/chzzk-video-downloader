package settings

import (
	"path/filepath"
	"testing"
)

func TestAuthenticationDefaults(t *testing.T) {
	value := AuthenticationDefaults()
	if value.Enabled || value.CookiesFilePath != "" {
		t.Fatalf("unexpected authentication defaults: %#v", value)
	}
	if err := ValidateAuthentication(value); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizeAuthentication(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cookies.txt")
	value := NormalizeAuthentication(AuthenticationSettings{
		Enabled: true, CookiesFilePath: " " + path + " ",
	})
	if value.CookiesFilePath != path {
		t.Fatalf("unexpected normalized authentication settings: %#v", value)
	}
}

func TestValidateAuthenticationRequiresCookiesFileWhenEnabled(t *testing.T) {
	value := AuthenticationDefaults()
	value.Enabled = true
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
	path := filepath.Join(t.TempDir(), "cookies.txt")
	updated, err := store.Update(AuthenticationSettings{
		Enabled: true, CookiesFilePath: path,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Enabled || updated.CookiesFilePath != path || store.Get() != updated {
		t.Fatalf("unexpected authentication store value: %#v", updated)
	}
}
