package updater

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseVersionSupportsReleaseTagFormats(t *testing.T) {
	for _, input := range []string{"0.1.4", "v0.1.4", "release-0.1.4", "release-v0.1.4"} {
		version, err := parseVersion(input)
		if err != nil {
			t.Fatalf("parseVersion(%q) failed: %v", input, err)
		}
		if got := version.String(); got != "0.1.4" {
			t.Fatalf("parseVersion(%q) = %q, want %q", input, got, "0.1.4")
		}
	}
}

func TestCheckerReportsNewerRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != "CHZZK-Video-Downloader/0.1.4" {
			t.Errorf("User-Agent = %q", got)
		}
		if got := r.Header.Get("Accept"); got != "application/vnd.github+json" {
			t.Errorf("Accept = %q", got)
		}
		_ = json.NewEncoder(w).Encode(githubRelease{
			TagName: "release-v0.1.5",
			Name:    "release-v0.1.5",
			HTMLURL: "https://github.com/MoonKim-isMe/chzzk-video-downloader/releases/tag/release-v0.1.5",
		})
	}))
	defer server.Close()

	checker := NewChecker(server.Client())
	checker.endpoint = server.URL
	result, err := checker.Check(context.Background(), "0.1.4")
	if err != nil {
		t.Fatal(err)
	}
	if !result.UpdateAvailable || result.LatestVersion != "0.1.5" || result.CurrentVersion != "0.1.4" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestCheckerDoesNotReportSameOrOlderRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(githubRelease{
			TagName: "release-v0.1.4",
			HTMLURL: "https://github.com/MoonKim-isMe/chzzk-video-downloader/releases/tag/release-v0.1.4",
		})
	}))
	defer server.Close()

	checker := NewChecker(server.Client())
	checker.endpoint = server.URL
	for _, current := range []string{"0.1.4", "0.1.5"} {
		result, err := checker.Check(context.Background(), current)
		if err != nil {
			t.Fatal(err)
		}
		if result.UpdateAvailable {
			t.Fatalf("current %s should not report an update: %+v", current, result)
		}
	}
}

func TestCheckerRejectsInvalidReleaseTag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(githubRelease{
			TagName: "latest",
			HTMLURL: "https://github.com/MoonKim-isMe/chzzk-video-downloader/releases/latest",
		})
	}))
	defer server.Close()

	checker := NewChecker(server.Client())
	checker.endpoint = server.URL
	if _, err := checker.Check(context.Background(), "0.1.4"); err == nil {
		t.Fatal("expected invalid release tag error")
	}
}
