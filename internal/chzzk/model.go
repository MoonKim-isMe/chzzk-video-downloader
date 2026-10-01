package chzzk

import "fmt"

type Channel struct {
	ChannelID          string `json:"channelId"`
	ChannelName        string `json:"channelName"`
	ChannelImageURL    string `json:"channelImageUrl"`
	ChannelDescription string `json:"channelDescription"`
	FollowerCount      int64  `json:"followerCount"`
	VerifiedMark       bool   `json:"verifiedMark"`
	OpenLive           bool   `json:"openLive"`
	ChannelURL         string `json:"channelUrl"`
}

type ChannelSearchResult struct {
	Channels   []Channel `json:"channels"`
	NextOffset int       `json:"nextOffset"`
	HasNext    bool      `json:"hasNext"`
}

func (c Channel) validate() error {
	if !channelIDPattern.MatchString(c.ChannelID) {
		return fmt.Errorf("유효하지 않은 채널 ID입니다")
	}
	if c.ChannelName == "" {
		return fmt.Errorf("채널 이름이 없습니다")
	}
	return nil
}

func normalizeChannel(channel apiChannel) Channel {
	return Channel{
		ChannelID:          channel.ChannelID,
		ChannelName:        channel.ChannelName,
		ChannelImageURL:    channel.ChannelImageURL,
		ChannelDescription: channel.ChannelDescription,
		FollowerCount:      channel.FollowerCount,
		VerifiedMark:       channel.VerifiedMark,
		OpenLive:           channel.OpenLive,
		ChannelURL:         ChannelURL(channel.ChannelID),
	}
}
