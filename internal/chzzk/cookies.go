package chzzk

import (
	"bufio"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type netscapeCookie struct {
	domain            string
	includeSubdomains bool
	path              string
	secure            bool
	expiresAt         int64
	name              string
	value             string
}

func applyCookiesFile(req *http.Request, rawPath string) error {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		return nil
	}
	if strings.ContainsRune(rawPath, '\x00') {
		return fmt.Errorf("cookies.txt 파일 경로가 올바르지 않습니다")
	}

	cleanPath := filepath.Clean(rawPath)
	if cleanPath == "." {
		return fmt.Errorf("cookies.txt 파일 경로가 올바르지 않습니다")
	}

	file, err := os.Open(cleanPath)
	if err != nil {
		return fmt.Errorf("cookies.txt 파일을 열 수 없습니다: %w", err)
	}
	defer file.Close()

	now := time.Now()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	for scanner.Scan() {
		cookie, ok := parseNetscapeCookieLine(scanner.Text())
		if !ok || !cookie.matches(req.URL, now) {
			continue
		}
		req.AddCookie(&http.Cookie{
			Name:  cookie.name,
			Value: cookie.value,
		})
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("cookies.txt 파일을 읽을 수 없습니다: %w", err)
	}
	return nil
}

func parseNetscapeCookieLine(rawLine string) (netscapeCookie, bool) {
	line := strings.TrimSuffix(rawLine, "\r")
	if strings.TrimSpace(line) == "" {
		return netscapeCookie{}, false
	}

	if strings.HasPrefix(line, "#HttpOnly_") {
		line = strings.TrimPrefix(line, "#HttpOnly_")
	} else if strings.HasPrefix(line, "#") {
		return netscapeCookie{}, false
	}

	fields := strings.SplitN(line, "\t", 7)
	if len(fields) != 7 {
		return netscapeCookie{}, false
	}

	domain := strings.TrimSpace(fields[0])
	name := strings.TrimSpace(fields[5])
	if domain == "" || name == "" {
		return netscapeCookie{}, false
	}

	expiresAt := int64(0)
	if value := strings.TrimSpace(fields[4]); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return netscapeCookie{}, false
		}
		expiresAt = parsed
	}

	cookiePath := strings.TrimSpace(fields[2])
	if cookiePath == "" {
		cookiePath = "/"
	}

	return netscapeCookie{
		domain:            domain,
		includeSubdomains: strings.EqualFold(strings.TrimSpace(fields[1]), "TRUE"),
		path:              cookiePath,
		secure:            strings.EqualFold(strings.TrimSpace(fields[3]), "TRUE"),
		expiresAt:         expiresAt,
		name:              name,
		value:             fields[6],
	}, true
}

func (c netscapeCookie) matches(target *url.URL, now time.Time) bool {
	host := strings.ToLower(target.Hostname())
	domain := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(c.domain)), ".")
	if host == "" || domain == "" {
		return false
	}

	if c.includeSubdomains {
		if host != domain && !strings.HasSuffix(host, "."+domain) {
			return false
		}
	} else if host != domain {
		return false
	}

	if c.secure && target.Scheme != "https" {
		return false
	}
	if c.expiresAt > 0 && c.expiresAt <= now.Unix() {
		return false
	}

	requestPath := target.EscapedPath()
	if requestPath == "" {
		requestPath = "/"
	}
	cookiePath := c.path
	if cookiePath == "" {
		cookiePath = "/"
	}
	if requestPath == cookiePath {
		return true
	}
	if !strings.HasPrefix(requestPath, cookiePath) {
		return false
	}
	if strings.HasSuffix(cookiePath, "/") {
		return true
	}
	return len(requestPath) > len(cookiePath) && requestPath[len(cookiePath)] == '/'
}
