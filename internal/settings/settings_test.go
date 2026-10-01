package settings

import (
	"path/filepath"
	"sync"
	"testing"
)

func TestDefaults(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "downloads")
	value := Defaults(dir)
	if value.DownloadDir != dir ||
		value.Resolution != ResolutionBest ||
		value.OutputFormat != OutputFormatMP4 ||
		value.MaxConcurrentDownloads != 1 ||
		value.Theme != ThemeDark {
		t.Fatalf("unexpected defaults: %#v", value)
	}
	if err := Validate(value); err != nil {
		t.Fatal(err)
	}
}

func TestStoreUpdateNormalizesValues(t *testing.T) {
	store, err := NewStore(Defaults(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}

	next, err := store.Update(AppSettings{
		DownloadDir:            filepath.Join(t.TempDir(), "video"),
		Resolution:             Resolution(" 1080P "),
		OutputFormat:           OutputFormat(" MKV "),
		MaxConcurrentDownloads: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	if next.Resolution != Resolution1080p ||
		next.OutputFormat != OutputFormatMKV ||
		next.MaxConcurrentDownloads != 4 ||
		next.Theme != ThemeDark {
		t.Fatalf("unexpected normalized settings: %#v", next)
	}
	if store.Get() != next {
		t.Fatalf("store mismatch: %#v", store.Get())
	}
}

func TestStoreRejectsInvalidUpdateWithoutChangingValue(t *testing.T) {
	store, err := NewStore(Defaults(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	before := store.Get()

	_, err = store.Update(AppSettings{
		DownloadDir:            t.TempDir(),
		Resolution:             ResolutionBest,
		OutputFormat:           OutputFormatMP4,
		MaxConcurrentDownloads: 9,
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if store.Get() != before {
		t.Fatalf("invalid update changed store: before=%#v after=%#v", before, store.Get())
	}
}

func TestValidateRejectsUnsupportedValues(t *testing.T) {
	base := Defaults(t.TempDir())
	cases := []AppSettings{
		func() AppSettings { value := base; value.DownloadDir = ""; return value }(),
		func() AppSettings { value := base; value.Resolution = "480p"; return value }(),
		func() AppSettings { value := base; value.OutputFormat = "avi"; return value }(),
		func() AppSettings { value := base; value.MaxConcurrentDownloads = 0; return value }(),
		func() AppSettings { value := base; value.Theme = "system"; return value }(),
	}

	for _, value := range cases {
		if err := Validate(value); err == nil {
			t.Fatalf("expected validation error for %#v", value)
		}
	}
}

func TestStoreConcurrentGetAndUpdate(t *testing.T) {
	store, err := NewStore(Defaults(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for index := 0; index < 20; index++ {
		wg.Add(2)

		go func() {
			defer wg.Done()
			if err := Validate(store.Get()); err != nil {
				t.Errorf("invalid read: %v", err)
			}
		}()

		go func(index int) {
			defer wg.Done()
			_, err := store.Update(AppSettings{
				DownloadDir:            t.TempDir(),
				Resolution:             Resolution1080p,
				OutputFormat:           OutputFormatMP4,
				MaxConcurrentDownloads: (index % MaxConcurrentDownloads) + 1,
			})
			if err != nil {
				t.Errorf("update: %v", err)
			}
		}(index)
	}
	wg.Wait()

	if err := Validate(store.Get()); err != nil {
		t.Fatal(err)
	}
}


func TestStoreUpdatesTheme(t *testing.T) {
	store, err := NewStore(Defaults(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}

	next := store.Get()
	next.Theme = ThemeLight
	updated, err := store.Update(next)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Theme != ThemeLight || store.Get().Theme != ThemeLight {
		t.Fatalf("theme was not updated: %#v", updated)
	}
}
