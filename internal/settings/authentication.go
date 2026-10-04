package settings

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
)

type AuthenticationMode string

const (
	AuthenticationModeBrowser     AuthenticationMode = "browser"
	AuthenticationModeCookiesFile AuthenticationMode = "cookies_file"
)

type AuthenticationBrowser string

const (
	AuthenticationBrowserChrome  AuthenticationBrowser = "chrome"
	AuthenticationBrowserEdge    AuthenticationBrowser = "edge"
	AuthenticationBrowserWhale   AuthenticationBrowser = "whale"
	AuthenticationBrowserFirefox AuthenticationBrowser = "firefox"
	AuthenticationBrowserBrave   AuthenticationBrowser = "brave"
	AuthenticationBrowserVivaldi AuthenticationBrowser = "vivaldi"
)

const (
	DefaultAuthenticationMode    = AuthenticationModeBrowser
	DefaultAuthenticationBrowser = AuthenticationBrowserChrome
)

type AuthenticationSettings struct {
	Enabled         bool                  `json:"enabled"`
	Mode            AuthenticationMode    `json:"mode"`
	Browser         AuthenticationBrowser `json:"browser"`
	BrowserProfile  string                `json:"browserProfile"`
	CookiesFilePath string                `json:"cookiesFilePath"`
}

func AuthenticationDefaults() AuthenticationSettings {
	return AuthenticationSettings{
		Enabled: false,
		Mode:    DefaultAuthenticationMode,
		Browser: DefaultAuthenticationBrowser,
	}
}

func NormalizeAuthentication(value AuthenticationSettings) AuthenticationSettings {
	value.Mode = AuthenticationMode(strings.ToLower(strings.TrimSpace(string(value.Mode))))
	if value.Mode == "" {
		value.Mode = DefaultAuthenticationMode
	}
	value.Browser = AuthenticationBrowser(strings.ToLower(strings.TrimSpace(string(value.Browser))))
	if value.Browser == "" {
		value.Browser = DefaultAuthenticationBrowser
	}
	value.BrowserProfile = strings.TrimSpace(value.BrowserProfile)
	value.CookiesFilePath = strings.TrimSpace(value.CookiesFilePath)
	if value.CookiesFilePath != "" {
		value.CookiesFilePath = filepath.Clean(value.CookiesFilePath)
	}
	return value
}

func ValidateAuthentication(value AuthenticationSettings) error {
	value = NormalizeAuthentication(value)
	switch value.Mode {
	case AuthenticationModeBrowser, AuthenticationModeCookiesFile:
	default:
		return fmt.Errorf("지원하지 않는 인증 방식입니다: %s", value.Mode)
	}
	switch value.Browser {
	case AuthenticationBrowserChrome, AuthenticationBrowserEdge, AuthenticationBrowserWhale,
		AuthenticationBrowserFirefox, AuthenticationBrowserBrave, AuthenticationBrowserVivaldi:
	default:
		return fmt.Errorf("지원하지 않는 인증 브라우저입니다: %s", value.Browser)
	}
	if strings.ContainsAny(value.BrowserProfile, "\x00\r\n") {
		return fmt.Errorf("브라우저 프로필 이름에 사용할 수 없는 문자가 포함되어 있습니다")
	}
	if strings.ContainsRune(value.CookiesFilePath, '\x00') {
		return fmt.Errorf("쿠키 파일 경로에 사용할 수 없는 문자가 포함되어 있습니다")
	}
	if value.Enabled && value.Mode == AuthenticationModeCookiesFile &&
		(value.CookiesFilePath == "" || value.CookiesFilePath == ".") {
		return fmt.Errorf("cookies.txt 파일을 선택해 주세요")
	}
	return nil
}

type AuthenticationStore struct {
	mu    sync.RWMutex
	value AuthenticationSettings
}

func NewAuthenticationStore(defaults AuthenticationSettings) (*AuthenticationStore, error) {
	defaults = NormalizeAuthentication(defaults)
	if err := ValidateAuthentication(defaults); err != nil {
		return nil, err
	}
	return &AuthenticationStore{value: defaults}, nil
}

func (s *AuthenticationStore) Get() AuthenticationSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.value
}

func (s *AuthenticationStore) Update(next AuthenticationSettings) (AuthenticationSettings, error) {
	next = NormalizeAuthentication(next)
	if err := ValidateAuthentication(next); err != nil {
		return AuthenticationSettings{}, err
	}
	s.mu.Lock()
	s.value = next
	s.mu.Unlock()
	return next, nil
}
