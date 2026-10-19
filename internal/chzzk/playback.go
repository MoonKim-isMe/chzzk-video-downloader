package chzzk

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// VideoPlaybackMetadata is used only while resolving an HLS fallback stream.
// InKey must never be persisted or printed in diagnostic logs.
type VideoPlaybackMetadata struct {
	VideoID  string
	InKey    string
	Title    string
	Duration int64
}

func (c *Client) GetVideoPlaybackMetadataWithCookiesFile(ctx context.Context, videoNo int64, cookiesFilePath string) (VideoPlaybackMetadata, error) {
	if videoNo <= 0 {
		return VideoPlaybackMetadata{}, fmt.Errorf("유효하지 않은 VOD 번호입니다")
	}
	var response apiEnvelope[struct {
		VideoID    string `json:"videoId"`
		InKey      string `json:"inKey"`
		VideoTitle string `json:"videoTitle"`
		Duration   int64  `json:"duration"`
	}]
	if err := c.getWithCookiesFile(ctx, "/service/v3/videos/"+strconv.FormatInt(videoNo, 10), &response, cookiesFilePath); err != nil {
		return VideoPlaybackMetadata{}, err
	}
	value := response.Content
	if strings.TrimSpace(value.VideoID) == "" || strings.TrimSpace(value.InKey) == "" {
		return VideoPlaybackMetadata{}, fmt.Errorf("재생 정보가 제공되지 않는 영상입니다")
	}
	return VideoPlaybackMetadata{VideoID: value.VideoID, InKey: value.InKey, Title: value.VideoTitle, Duration: value.Duration}, nil
}
