package downloader

import (
	"fmt"
	"path/filepath"
	"strings"
)

const (
	authenticationModeBrowser     = "browser"
	authenticationModeCookiesFile = "cookies_file"
)

type AuthenticationOptions struct {
	Mode            string `json:"mode,omitempty"`
	Browser         string `json:"browser,omitempty"`
	BrowserProfile  string `json:"browserProfile,omitempty"`
	CookiesFilePath string `json:"cookiesFilePath,omitempty"`
}

func appendAuthenticationArgs(args []string, options AuthenticationOptions) ([]string, error) {
	authArgs, err := authenticationArgs(options)
	if err != nil {
		return nil, err
	}
	return append(args, authArgs...), nil
}

func authenticationArgs(options AuthenticationOptions) ([]string, error) {
	mode := strings.ToLower(strings.TrimSpace(options.Mode))
	switch mode {
	case "":
		return nil, nil
	case authenticationModeBrowser:
		browser := strings.ToLower(strings.TrimSpace(options.Browser))
		switch browser {
		case "chrome", "edge", "whale", "firefox", "brave", "vivaldi":
		default:
			return nil, fmt.Errorf("지원하지 않는 인증 브라우저입니다: %s", browser)
		}
		profile := strings.TrimSpace(options.BrowserProfile)
		if strings.ContainsAny(profile, "\x00\r\n") {
			return nil, fmt.Errorf("브라우저 프로필 이름에 사용할 수 없는 문자가 포함되어 있습니다")
		}
		source := browser
		if profile != "" {
			source += ":" + profile
		}
		return []string{"--cookies-from-browser", source}, nil
	case authenticationModeCookiesFile:
		path := strings.TrimSpace(options.CookiesFilePath)
		if path == "" || strings.ContainsRune(path, '\x00') {
			return nil, fmt.Errorf("유효한 cookies.txt 파일 경로가 필요합니다")
		}
		path = filepath.Clean(path)
		if path == "." {
			return nil, fmt.Errorf("cookies.txt 파일 경로가 필요합니다")
		}
		return []string{"--cookies", path}, nil
	default:
		return nil, fmt.Errorf("지원하지 않는 다운로드 인증 방식입니다: %s", mode)
	}
}
