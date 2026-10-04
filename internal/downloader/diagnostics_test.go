package downloader

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteDownloadFailureLogCapturesDiagnosticsAndRedactsSecrets(t *testing.T) {
	outputDir := t.TempDir()
	request := DownloadRequest{
		URL:            "https://chzzk.naver.com/video/12345",
		OutputDir:      outputDir,
		FormatSelector: "bv*+ba/b",
		OutputFormat:   "mp4",
	}
	spec := CommandSpec{
		Path: "yt-dlp.exe",
		Args: []string{
			"--password", "secret-password",
			"--proxy=https://user:pass@example.com",
			request.URL,
		},
	}
	processErr := &ProcessError{
		Program:    "yt-dlp.exe",
		ExitCode:   1,
		StderrTail: []string{"ERROR: manifest failed"},
		Err:        errors.New("exit status 1"),
	}
	downloadErr := &DownloadFailure{
		Kind:    DownloadFailurePartialDataConflict,
		Message: "사용자 오류 메시지",
		Cause:   processErr,
	}
	result := DownloadResult{
		LastProgress: DownloadProgress{
			Status:          "downloading",
			Percent:         42.5,
			DownloadedBytes: 425,
			TotalBytes:      1000,
			ETASeconds:      5,
		},
		DiagnosticLines: []OutputLine{
			{Stream: StreamStdout, Text: "download output"},
			{Stream: StreamStderr, Text: "ERROR: raw failure detail"},
			{Stream: StreamStderr, Text: `[debug] Invoking ffmpeg downloader on "https://example.com/master.m3u8?token=secret-token&expires=9999"`},
		},
	}

	logPath, err := writeDownloadFailureLog(request, spec, result, downloadErr)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(logPath) != filepath.Join(outputDir, downloadLogDirectoryName) {
		t.Fatalf("unexpected log directory: %s", logPath)
	}

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, expected := range []string{
		"video_no: 12345",
		"error_code: partial_data_conflict",
		"ERROR: raw failure detail",
		"exit code 1",
		"--password <redacted>",
		"--proxy=<redacted>",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("missing %q in log:\n%s", expected, text)
		}
	}
	if strings.Contains(text, "secret-password") ||
		strings.Contains(text, "user:pass") ||
		strings.Contains(text, "secret-token") ||
		strings.Contains(text, "expires=9999") {
		t.Fatalf("sensitive diagnostic data leaked into log:\n%s", text)
	}
	if !strings.Contains(text, "https://example.com/master.m3u8?<redacted>") {
		t.Fatalf("redacted diagnostic URL missing from log:\n%s", text)
	}
}

func TestRedactDiagnosticTextRedactsAuthenticationArguments(t *testing.T) {
	inputs := []string{
		`[debug] Command-line config: ['--cookies', 'C:\\Users\\tester\\cookies.txt']`,
		`yt-dlp --cookies "C:\\Users\\tester\\cookies.txt" https://example.com/video`,
	}
	for _, input := range inputs {
		got := redactDiagnosticText(input)
		if strings.Contains(got, "cookies.txt") {
			t.Fatalf("authentication argument leaked: %q", got)
		}
		if !strings.Contains(got, "<redacted>") {
			t.Fatalf("redaction marker missing: %q", got)
		}
	}
}

func TestRedactDiagnosticTextRedactsURLQueryAndFragment(t *testing.T) {
	input := `[debug] ffmpeg command line: https://cdn.example.com/video.m3u8?token=abc#fragment`
	got := redactDiagnosticText(input)
	if strings.Contains(got, "token=abc") || strings.Contains(got, "fragment") {
		t.Fatalf("sensitive URL data was not redacted: %q", got)
	}
	if !strings.Contains(got, "https://cdn.example.com/video.m3u8?<redacted>#<redacted>") {
		t.Fatalf("unexpected redacted URL: %q", got)
	}
}

func TestRedactDiagnosticTextRedactsSignedCDNPathToken(t *testing.T) {
	input := `[https] Opening 'https://cdn.example.com/chzzk/1080p/hdntl=exp=123~acl=*/kr/*~data=hdntl~hmac=secret-signature/segment.m4v' for reading`
	got := redactDiagnosticText(input)
	if strings.Contains(got, "secret-signature") ||
		strings.Contains(got, "exp=123") ||
		strings.Contains(got, "~hmac=") {
		t.Fatalf("signed CDN path token was not redacted: %q", got)
	}
	if !strings.Contains(got, "https://cdn.example.com/chzzk/1080p/<redacted>/segment.m4v") {
		t.Fatalf("unexpected signed path redaction: %q", got)
	}
}

func TestAppendDiagnosticLineKeepsLatestLines(t *testing.T) {
	lines := make([]OutputLine, 0, maxDiagnosticLines)
	for index := 0; index < maxDiagnosticLines+5; index++ {
		lines = appendDiagnosticLine(lines, OutputLine{
			Stream: StreamStderr,
			Text:   string(rune('A' + index%26)),
		})
	}
	if len(lines) != maxDiagnosticLines {
		t.Fatalf("unexpected diagnostic line count: %d", len(lines))
	}
}
