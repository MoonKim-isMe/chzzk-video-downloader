package chzzk

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var channelIDPattern = regexp.MustCompile(`^[a-fA-F0-9]{32}$`)

func ChannelURL(channelID string) string {
	return "https://chzzk.naver.com/" + strings.ToLower(channelID)
}

func ParseChannelURL(raw string) (string, string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", "", fmt.Errorf("채널 URL을 입력해 주세요")
	}
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return "", "", fmt.Errorf("채널 URL 형식이 올바르지 않습니다")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", "", fmt.Errorf("지원하지 않는 URL 형식입니다")
	}
	if !strings.EqualFold(parsed.Hostname(), "chzzk.naver.com") {
		return "", "", fmt.Errorf("치지직 채널 URL만 사용할 수 있습니다")
	}

	path := strings.Trim(parsed.Path, "/")
	parts := strings.Split(path, "/")
	if len(parts) != 1 || !channelIDPattern.MatchString(parts[0]) {
		return "", "", fmt.Errorf("채널 ID가 포함된 URL을 입력해 주세요")
	}

	channelID := strings.ToLower(parts[0])
	return channelID, ChannelURL(channelID), nil
}
