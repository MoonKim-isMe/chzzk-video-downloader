package chzzk

import (
 "context"
 "fmt"
 "strings"
)

// LiveStatus is the minimal broadcast identity used for auto recording.
// A non-open response must never be treated as a successful recording.
type LiveStatus struct {
 ChannelID string `json:"channelId"`
 ChannelName string `json:"channelName"`
 LiveID int64 `json:"liveId"`
 LiveTitle string `json:"liveTitle"`
 OpenDate string `json:"openDate"`
 IsLive bool `json:"isLive"`
}

func (c *Client) GetLiveStatusWithCookiesFile(ctx context.Context, channelID, cookiesFilePath string) (LiveStatus, error) {
 channelID = strings.ToLower(strings.TrimSpace(channelID))
 if !channelIDPattern.MatchString(channelID) { return LiveStatus{}, fmt.Errorf("유효하지 않은 채널 ID입니다") }
 var response apiEnvelope[struct {
   Status string `json:"status"`
   LiveID int64 `json:"liveId"`
   LiveTitle string `json:"liveTitle"`
   OpenDate string `json:"openDate"`
   Channel struct { ChannelName string `json:"channelName"` } `json:"channel"`
 }]
 if err := c.getWithCookiesFile(ctx, "/service/v3/channels/"+channelID+"/live-detail", &response, cookiesFilePath); err != nil { return LiveStatus{}, err }
 v:=response.Content
 return LiveStatus{ChannelID:channelID,ChannelName:v.Channel.ChannelName,LiveID:v.LiveID,LiveTitle:v.LiveTitle,OpenDate:v.OpenDate,IsLive:strings.EqualFold(v.Status,"OPEN") && v.LiveID>0},nil
}
