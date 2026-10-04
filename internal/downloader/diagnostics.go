package downloader

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	downloadLogDirectoryName = ".chzzk-logs"
	maxDiagnosticLines       = 400
)

var (
	diagnosticURLPattern = regexp.MustCompile(`https?://[^\s"'<>]+`)
	diagnosticSensitiveOptionPattern = regexp.MustCompile(
		`(?i)(--(?:cookies|cookies-from-browser|username|password|proxy|netrc-location))(?:=|\s+)(?:"[^"]*"|'[^']*'|[^\s,\]]+)`,
	)
	diagnosticSensitiveListOptionPattern = regexp.MustCompile(
		`(?i)(['"]--(?:cookies|cookies-from-browser|username|password|proxy|netrc-location)['"]\s*,\s*)(['"][^'"]*['"])`,
	)
)

func appendDiagnosticLine(lines []OutputLine, line OutputLine) []OutputLine {
	line.Text = redactDiagnosticText(line.Text)
	if len(lines) >= maxDiagnosticLines {
		copy(lines, lines[1:])
		lines[len(lines)-1] = line
		return lines
	}
	return append(lines, line)
}

func writeDownloadFailureLog(
	request DownloadRequest,
	spec CommandSpec,
	result DownloadResult,
	downloadErr error,
) (string, error) {
	outputDir, err := normalizeOutputDir(request.OutputDir)
	if err != nil {
		return "", err
	}
	videoURL, err := normalizeVideoURL(request.URL)
	if err != nil {
		videoURL = strings.TrimSpace(request.URL)
	}
	videoNo, _ := videoNoFromNormalizedURL(videoURL)

	logDir := filepath.Join(outputDir, downloadLogDirectoryName)
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return "", fmt.Errorf("다운로드 오류 로그 폴더를 만들 수 없습니다: %w", err)
	}

	timestamp := time.Now().UTC()
	fileName := fmt.Sprintf(
		"vod-%d-%s.log",
		videoNo,
		timestamp.Format("20060102T150405.000000000Z"),
	)
	logPath := filepath.Join(logDir, fileName)

	var builder strings.Builder
	fmt.Fprintln(&builder, "CHZZK Video Downloader - Download Failure Log")
	fmt.Fprintf(&builder, "timestamp_utc: %s\n", timestamp.Format(time.RFC3339Nano))
	fmt.Fprintf(&builder, "video_no: %d\n", videoNo)
	fmt.Fprintf(&builder, "video_url: %s\n", redactDiagnosticText(videoURL))
	fmt.Fprintf(&builder, "output_dir: %s\n", outputDir)
	fmt.Fprintf(&builder, "format_selector: %s\n", strings.TrimSpace(request.FormatSelector))
	fmt.Fprintf(&builder, "output_format: %s\n", strings.TrimSpace(request.OutputFormat))
	if kind, ok := downloadFailureKind(downloadErr); ok {
		fmt.Fprintf(&builder, "error_code: %s\n", kind)
	}
	fmt.Fprintf(&builder, "error: %s\n", redactDiagnosticText(downloadErr.Error()))
	fmt.Fprintf(
		&builder,
		"last_progress: status=%s percent=%.1f downloaded=%d total=%d speed=%.0f eta=%d\n",
		result.LastProgress.Status,
		result.LastProgress.Percent,
		result.LastProgress.DownloadedBytes,
		result.LastProgress.TotalBytes,
		result.LastProgress.SpeedBytesPerSecond,
		result.LastProgress.ETASeconds,
	)
	if strings.TrimSpace(spec.Path) != "" {
		fmt.Fprintf(&builder, "command: %s %s\n", spec.Path, strings.Join(redactCommandArgs(spec.Args), " "))
	}

	fmt.Fprintln(&builder, "")
	fmt.Fprintln(&builder, "[error chain]")
	for depth, current := 0, downloadErr; current != nil && depth < 12; depth++ {
		fmt.Fprintf(
			&builder,
			"%d: %T: %s\n",
			depth,
			current,
			redactDiagnosticText(current.Error()),
		)
		current = errors.Unwrap(current)
	}

	fmt.Fprintln(&builder, "")
	fmt.Fprintln(&builder, "[recent process output]")
	if len(result.DiagnosticLines) == 0 {
		fmt.Fprintln(&builder, "(no process output captured)")
	} else {
		for _, line := range result.DiagnosticLines {
			fmt.Fprintf(&builder, "[%s] %s\n", line.Stream, redactDiagnosticText(line.Text))
		}
	}

	if err := os.WriteFile(logPath, []byte(builder.String()), 0o644); err != nil {
		return "", fmt.Errorf("다운로드 오류 로그를 저장할 수 없습니다: %w", err)
	}
	return logPath, nil
}

func redactCommandArgs(args []string) []string {
	redacted := append([]string(nil), args...)
	sensitive := map[string]struct{}{
		"--cookies":              {},
		"--cookies-from-browser": {},
		"--username":             {},
		"--password":             {},
		"--proxy":                {},
		"--netrc-location":       {},
	}

	for index := 0; index < len(redacted); index++ {
		value := redacted[index]
		if _, ok := sensitive[value]; ok {
			if index+1 < len(redacted) {
				redacted[index+1] = "<redacted>"
				index++
			}
			continue
		}
		for flag := range sensitive {
			prefix := flag + "="
			if strings.HasPrefix(value, prefix) {
				redacted[index] = prefix + "<redacted>"
				break
			}
		}
	}

	return redacted
}


func redactDiagnosticText(text string) string {
	text = diagnosticSensitiveListOptionPattern.ReplaceAllString(text, "$1'<redacted>'")
	text = diagnosticSensitiveOptionPattern.ReplaceAllString(text, "$1 <redacted>")

	return diagnosticURLPattern.ReplaceAllStringFunc(text, func(raw string) string {
		core, suffix := splitDiagnosticURLSuffix(raw)
		parsed, err := url.Parse(core)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return raw
		}

		safePath := redactDiagnosticURLPath(parsed.EscapedPath())
		safe := parsed.Scheme + "://" + parsed.Host + safePath
		if parsed.RawQuery != "" {
			safe += "?<redacted>"
		}
		if parsed.Fragment != "" {
			safe += "#<redacted>"
		}
		return safe + suffix
	})
}

func splitDiagnosticURLSuffix(raw string) (string, string) {
	index := len(raw)
	for index > 0 {
		switch raw[index-1] {
		case ')', ']', '}', ',', ';', '.':
			index--
		default:
			return raw[:index], raw[index:]
		}
	}
	return raw, ""
}


func redactDiagnosticURLPath(path string) string {
	segments := strings.Split(path, "/")
	for index, segment := range segments {
		lower := strings.ToLower(segment)
		if strings.HasPrefix(lower, "hdntl=") ||
			strings.HasPrefix(lower, "hdnts=") ||
			strings.Contains(lower, "~hmac=") {
			segments[index] = "<redacted>"
		}
	}
	return strings.Join(segments, "/")
}
