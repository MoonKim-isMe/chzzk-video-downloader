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
	if strings.Contains(text, "secret-password") || strings.Contains(text, "user:pass") {
		t.Fatalf("sensitive command argument leaked into log:\n%s", text)
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
