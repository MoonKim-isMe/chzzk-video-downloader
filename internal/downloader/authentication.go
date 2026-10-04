package downloader

import (
	"fmt"
	"path/filepath"
	"strings"
)

type AuthenticationOptions struct {
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
	path := strings.TrimSpace(options.CookiesFilePath)
	if path == "" {
		return nil, nil
	}
	if strings.ContainsRune(path, '\x00') {
		return nil, fmt.Errorf("유효한 cookies.txt 파일 경로가 필요합니다")
	}
	path = filepath.Clean(path)
	if path == "." {
		return nil, fmt.Errorf("cookies.txt 파일 경로가 필요합니다")
	}
	return []string{"--cookies", path}, nil
}
