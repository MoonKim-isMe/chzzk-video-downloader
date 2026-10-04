package settings

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
)

type AuthenticationSettings struct {
	Enabled         bool   `json:"enabled"`
	CookiesFilePath string `json:"cookiesFilePath"`
}

func AuthenticationDefaults() AuthenticationSettings {
	return AuthenticationSettings{Enabled: false}
}

func NormalizeAuthentication(value AuthenticationSettings) AuthenticationSettings {
	value.CookiesFilePath = strings.TrimSpace(value.CookiesFilePath)
	if value.CookiesFilePath != "" {
		value.CookiesFilePath = filepath.Clean(value.CookiesFilePath)
	}
	return value
}

func ValidateAuthentication(value AuthenticationSettings) error {
	value = NormalizeAuthentication(value)
	if strings.ContainsRune(value.CookiesFilePath, '\x00') {
		return fmt.Errorf("쿠키 파일 경로에 사용할 수 없는 문자가 포함되어 있습니다")
	}
	if value.Enabled && (value.CookiesFilePath == "" || value.CookiesFilePath == ".") {
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
