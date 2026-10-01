package chzzk

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const (
	defaultVideoPageSize = 24
	maxVideoPageSize     = 50
)

type VideoChannel struct {
	ChannelID       string `json:"channelId"`
	ChannelName     string `json:"channelName"`
	ChannelImageURL string `json:"channelImageUrl"`
	VerifiedMark    bool   `json:"verifiedMark"`
}

type Video struct {
	VideoNo            int64        `json:"videoNo"`
	VideoID            string       `json:"videoId"`
	VideoTitle         string       `json:"videoTitle"`
	VideoType          string       `json:"videoType"`
	PublishDate        string       `json:"publishDate"`
	PublishDateAt      int64        `json:"publishDateAt"`
	ThumbnailImageURL  string       `json:"thumbnailImageUrl"`
	TrailerURL         string       `json:"trailerUrl"`
	Duration           int64        `json:"duration"`
	ReadCount          int64        `json:"readCount"`
	LivePV             int64        `json:"livePv"`
	CategoryType       string       `json:"categoryType"`
	VideoCategory      string       `json:"videoCategory"`
	VideoCategoryValue string       `json:"videoCategoryValue"`
	Exposure           bool         `json:"exposure"`
	Adult              bool         `json:"adult"`
	ClipActive         bool         `json:"clipActive"`
	CommentActive      bool         `json:"commentActive"`
	ChapterActive      bool         `json:"chapterActive"`
	Tags               []string     `json:"tags"`
	Channel            VideoChannel `json:"channel"`
	VideoURL           string       `json:"videoUrl"`
}

type VideoListResult struct {
	Videos     []Video `json:"videos"`
	Page       int     `json:"page"`
	Size       int     `json:"size"`
	TotalCount int64   `json:"totalCount"`
	TotalPages int     `json:"totalPages"`
	HasNext    bool    `json:"hasNext"`
	NextPage   int     `json:"nextPage"`
}

type apiVideo struct {
	VideoNo            int64      `json:"videoNo"`
	VideoID            string     `json:"videoId"`
	VideoTitle         string     `json:"videoTitle"`
	VideoType          string     `json:"videoType"`
	PublishDate        string     `json:"publishDate"`
	PublishDateAt      int64      `json:"publishDateAt"`
	ThumbnailImageURL  string     `json:"thumbnailImageUrl"`
	TrailerURL         string     `json:"trailerUrl"`
	Duration           int64      `json:"duration"`
	ReadCount          int64      `json:"readCount"`
	LivePV             int64      `json:"livePv"`
	CategoryType       string     `json:"categoryType"`
	VideoCategory      string     `json:"videoCategory"`
	VideoCategoryValue string     `json:"videoCategoryValue"`
	Exposure           bool       `json:"exposure"`
	Adult              bool       `json:"adult"`
	ClipActive         bool       `json:"clipActive"`
	CommentActive      bool       `json:"commentActive"`
	ChapterActive      bool       `json:"chapterActive"`
	Tags               []string   `json:"tags"`
	Channel            apiChannel `json:"channel"`
}

type videoListContent struct {
	Page       int        `json:"page"`
	Size       int        `json:"size"`
	TotalCount int64      `json:"totalCount"`
	TotalPages int        `json:"totalPages"`
	Data       []apiVideo `json:"data"`
}

func VideoURL(videoNo int64) string {
	return "https://chzzk.naver.com/video/" + strconv.FormatInt(videoNo, 10)
}

func (c *Client) GetChannelVideos(ctx context.Context, channelID string, page, size int) (VideoListResult, error) {
	channelID = strings.ToLower(strings.TrimSpace(channelID))
	if !channelIDPattern.MatchString(channelID) {
		return VideoListResult{}, fmt.Errorf("유효하지 않은 채널 ID입니다")
	}
	if page < 0 {
		page = 0
	}
	if size <= 0 {
		size = defaultVideoPageSize
	}
	if size > maxVideoPageSize {
		size = maxVideoPageSize
	}

	query := url.Values{}
	query.Set("sortType", "LATEST")
	query.Set("pagingType", "PAGE")
	query.Set("page", strconv.Itoa(page))
	query.Set("size", strconv.Itoa(size))
	query.Set("publishDateAt", "")
	query.Set("videoType", "")

	var response apiEnvelope[videoListContent]
	path := "/service/v1/channels/" + channelID + "/videos?" + query.Encode()
	if err := c.get(ctx, path, &response); err != nil {
		return VideoListResult{}, err
	}

	videos := make([]Video, 0, len(response.Content.Data))
	for _, item := range response.Content.Data {
		if item.VideoNo <= 0 || strings.TrimSpace(item.VideoTitle) == "" {
			continue
		}
		videos = append(videos, normalizeVideo(item))
	}
	if len(response.Content.Data) > 0 && len(videos) == 0 {
		return VideoListResult{}, fmt.Errorf("치지직 VOD API 응답 형식이 변경되었을 수 있습니다")
	}

	result := VideoListResult{
		Videos:     videos,
		Page:       response.Content.Page,
		Size:       response.Content.Size,
		TotalCount: response.Content.TotalCount,
		TotalPages: response.Content.TotalPages,
	}
	if result.Size <= 0 {
		result.Size = size
	}
	if result.Page < 0 {
		result.Page = page
	}
	if result.TotalPages <= 0 && result.TotalCount > 0 && result.Size > 0 {
		result.TotalPages = int((result.TotalCount + int64(result.Size) - 1) / int64(result.Size))
	}
	if result.TotalPages > 0 && result.Page+1 < result.TotalPages {
		result.HasNext = true
		result.NextPage = result.Page + 1
	}

	return result, nil
}

func normalizeVideo(video apiVideo) Video {
	tags := video.Tags
	if tags == nil {
		tags = []string{}
	}

	return Video{
		VideoNo:            video.VideoNo,
		VideoID:            video.VideoID,
		VideoTitle:         video.VideoTitle,
		VideoType:          video.VideoType,
		PublishDate:        video.PublishDate,
		PublishDateAt:      video.PublishDateAt,
		ThumbnailImageURL:  video.ThumbnailImageURL,
		TrailerURL:         video.TrailerURL,
		Duration:           video.Duration,
		ReadCount:          video.ReadCount,
		LivePV:             video.LivePV,
		CategoryType:       video.CategoryType,
		VideoCategory:      video.VideoCategory,
		VideoCategoryValue: video.VideoCategoryValue,
		Exposure:           video.Exposure,
		Adult:              video.Adult,
		ClipActive:         video.ClipActive,
		CommentActive:      video.CommentActive,
		ChapterActive:      video.ChapterActive,
		Tags:               tags,
		Channel: VideoChannel{
			ChannelID:       video.Channel.ChannelID,
			ChannelName:     video.Channel.ChannelName,
			ChannelImageURL: video.Channel.ChannelImageURL,
			VerifiedMark:    video.Channel.VerifiedMark,
		},
		VideoURL: VideoURL(video.VideoNo),
	}
}
