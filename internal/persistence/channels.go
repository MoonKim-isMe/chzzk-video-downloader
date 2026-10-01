package persistence

import (
	"fmt"
	"strings"
	"time"

	"github.com/MoonKim-isMe/chzzk-video-downloader/internal/chzzk"
)

func (d *Database) ListChannels() ([]chzzk.Channel, error) {
	rows, err := d.db.Query(`SELECT
		channel_id,
		channel_name,
		channel_image_url,
		channel_description,
		follower_count,
		verified_mark,
		open_live,
		channel_url
	FROM saved_channels
	ORDER BY LOWER(channel_name), channel_id`)
	if err != nil {
		return nil, fmt.Errorf("저장 채널을 조회할 수 없습니다: %w", err)
	}
	defer rows.Close()

	channels := make([]chzzk.Channel, 0)
	for rows.Next() {
		var channel chzzk.Channel
		var verified, openLive int
		if err := rows.Scan(
			&channel.ChannelID,
			&channel.ChannelName,
			&channel.ChannelImageURL,
			&channel.ChannelDescription,
			&channel.FollowerCount,
			&verified,
			&openLive,
			&channel.ChannelURL,
		); err != nil {
			return nil, fmt.Errorf("저장 채널 데이터를 읽을 수 없습니다: %w", err)
		}
		channel.VerifiedMark = scanBool(verified)
		channel.OpenLive = scanBool(openLive)
		channels = append(channels, channel)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("저장 채널 조회를 완료할 수 없습니다: %w", err)
	}
	return channels, nil
}

func (d *Database) UpsertChannel(channel chzzk.Channel) error {
	channel.ChannelID = strings.ToLower(strings.TrimSpace(channel.ChannelID))
	_, err := d.db.Exec(`INSERT INTO saved_channels (
		channel_id,
		channel_name,
		channel_image_url,
		channel_description,
		follower_count,
		verified_mark,
		open_live,
		channel_url,
		saved_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(channel_id) DO UPDATE SET
		channel_name = excluded.channel_name,
		channel_image_url = excluded.channel_image_url,
		channel_description = excluded.channel_description,
		follower_count = excluded.follower_count,
		verified_mark = excluded.verified_mark,
		open_live = excluded.open_live,
		channel_url = excluded.channel_url`,
		channel.ChannelID,
		channel.ChannelName,
		channel.ChannelImageURL,
		channel.ChannelDescription,
		channel.FollowerCount,
		boolInt(channel.VerifiedMark),
		boolInt(channel.OpenLive),
		channel.ChannelURL,
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("채널을 SQLite에 저장할 수 없습니다: %w", err)
	}
	return nil
}

func (d *Database) DeleteChannel(channelID string) error {
	channelID = strings.ToLower(strings.TrimSpace(channelID))
	if _, err := d.db.Exec("DELETE FROM saved_channels WHERE channel_id = ?", channelID); err != nil {
		return fmt.Errorf("저장 채널을 SQLite에서 삭제할 수 없습니다: %w", err)
	}
	return nil
}
