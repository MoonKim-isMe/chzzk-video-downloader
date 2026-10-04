package chzzk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBaseURL  = "https://api.chzzk.naver.com"
	defaultPageSize = 20
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

type apiChannel struct {
	ChannelID          string `json:"channelId"`
	ChannelName        string `json:"channelName"`
	ChannelImageURL    string `json:"channelImageUrl"`
	ChannelDescription string `json:"channelDescription"`
	FollowerCount      int64  `json:"followerCount"`
	VerifiedMark       bool   `json:"verifiedMark"`
	OpenLive           bool   `json:"openLive"`
}

type apiEnvelope[T any] struct {
	Code    int     `json:"code"`
	Message *string `json:"message"`
	Content T       `json:"content"`
}

type searchContent struct {
	Size int `json:"size"`
	Page struct {
		Next *struct {
			Offset int `json:"offset"`
		} `json:"next"`
	} `json:"page"`
	Data []struct {
		Channel apiChannel `json:"channel"`
	} `json:"data"`
}

func NewClient() *Client {
	return &Client{
		baseURL: defaultBaseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func newClient(baseURL string, httpClient *http.Client) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: httpClient,
	}
}

func (c *Client) SearchChannels(ctx context.Context, keyword string, offset, size int) (ChannelSearchResult, error) {
	return c.searchChannels(ctx, keyword, offset, size, "")
}

func (c *Client) SearchChannelsWithCookiesFile(
	ctx context.Context,
	keyword string,
	offset, size int,
	cookiesFilePath string,
) (ChannelSearchResult, error) {
	return c.searchChannels(ctx, keyword, offset, size, cookiesFilePath)
}

func (c *Client) searchChannels(
	ctx context.Context,
	keyword string,
	offset, size int,
	cookiesFilePath string,
) (ChannelSearchResult, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return ChannelSearchResult{}, fmt.Errorf("검색어를 입력해 주세요")
	}
	if offset < 0 {
		offset = 0
	}
	if size <= 0 {
		size = defaultPageSize
	}
	if size > 50 {
		size = 50
	}

	query := url.Values{}
	query.Set("keyword", keyword)
	query.Set("offset", strconv.Itoa(offset))
	query.Set("size", strconv.Itoa(size))
	query.Set("withFirstChannelContent", "false")

	var response apiEnvelope[searchContent]
	if err := c.getWithCookiesFile(ctx, "/service/v1/search/channels?"+query.Encode(), &response, cookiesFilePath); err != nil {
		return ChannelSearchResult{}, err
	}

	channels := make([]Channel, 0, len(response.Content.Data))
	for _, item := range response.Content.Data {
		channel := normalizeChannel(item.Channel)
		if err := channel.validate(); err != nil {
			continue
		}
		channels = append(channels, channel)
	}

	result := ChannelSearchResult{Channels: channels}
	if response.Content.Page.Next != nil {
		result.HasNext = true
		result.NextOffset = response.Content.Page.Next.Offset
	}
	return result, nil
}

func (c *Client) GetChannel(ctx context.Context, channelID string) (Channel, error) {
	channelID = strings.ToLower(strings.TrimSpace(channelID))
	if !channelIDPattern.MatchString(channelID) {
		return Channel{}, fmt.Errorf("유효하지 않은 채널 ID입니다")
	}

	var response apiEnvelope[apiChannel]
	if err := c.get(ctx, "/service/v1/channels/"+channelID, &response); err != nil {
		return Channel{}, err
	}

	channel := normalizeChannel(response.Content)
	if err := channel.validate(); err != nil {
		return Channel{}, fmt.Errorf("채널 정보를 확인할 수 없습니다")
	}
	return channel, nil
}

func (c *Client) get(ctx context.Context, path string, target any) error {
	return c.getWithCookiesFile(ctx, path, target, "")
}

func (c *Client) getWithCookiesFile(
	ctx context.Context,
	path string,
	target any,
	cookiesFilePath string,
) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("요청을 만들 수 없습니다: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "CHZZK-Video-Downloader/0.1")
	req.Header.Set("Origin", "https://chzzk.naver.com")
	req.Header.Set("Referer", "https://chzzk.naver.com/")
	if err := applyCookiesFile(req, cookiesFilePath); err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("치지직 API 요청에 실패했습니다: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("치지직 API가 HTTP %d를 반환했습니다", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return fmt.Errorf("치지직 API 응답을 읽을 수 없습니다: %w", err)
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("치지직 API 응답 형식이 변경되었을 수 있습니다: %w", err)
	}

	var status struct {
		Code    int     `json:"code"`
		Message *string `json:"message"`
	}
	if err := json.Unmarshal(body, &status); err == nil && status.Code != 0 && status.Code != http.StatusOK {
		message := "치지직 API 요청이 실패했습니다"
		if status.Message != nil && strings.TrimSpace(*status.Message) != "" {
			message = *status.Message
		}
		return fmt.Errorf("%s (code=%d)", message, status.Code)
	}

	return nil
}
