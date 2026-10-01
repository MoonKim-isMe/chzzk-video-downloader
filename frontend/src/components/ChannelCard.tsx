import { Avatar, Card, Space, Tag, Tooltip, Typography } from 'antd';
import type { KeyboardEvent, MouseEvent } from 'react';

import type { Channel } from '../types/channel';

const { Paragraph, Text } = Typography;
const followerFormatter = new Intl.NumberFormat('ko-KR');

function StarIcon({ filled }: { filled: boolean }) {
  return (
    <svg
      aria-hidden="true"
      viewBox="0 0 24 24"
      width="17"
      height="17"
      fill={filled ? 'currentColor' : 'none'}
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <path d="m12 3.6 2.57 5.2 5.74.84-4.15 4.04.98 5.72L12 16.7 6.86 19.4l.98-5.72-4.15-4.04 5.74-.84L12 3.6Z" />
    </svg>
  );
}

interface ChannelCardProps {
  channel: Channel;
  saved: boolean;
  selected?: boolean;
  onSelect: (channel: Channel) => void;
  onSave: (channel: Channel) => void;
  onRemove: (channelId: string) => void;
}

function ChannelCard({
  channel,
  saved,
  selected = false,
  onSelect,
  onSave,
  onRemove,
}: ChannelCardProps) {
  const select = () => onSelect(channel);

  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault();
      select();
    }
  };

  const handleBookmark = (event: MouseEvent<HTMLButtonElement>) => {
    event.stopPropagation();
    if (saved) {
      onRemove(channel.channelId);
      return;
    }
    onSave(channel);
  };

  return (
    <Card
      size="small"
      bordered={false}
      role="button"
      tabIndex={0}
      aria-pressed={selected}
      className={`channel-result-card ${selected ? 'is-selected' : ''}`}
      styles={{ body: { padding: 12 } }}
      onClick={select}
      onKeyDown={handleKeyDown}
    >
      <Tooltip title={saved ? '북마크 해제' : '북마크 추가'} placement="left">
        <button
          type="button"
          aria-label={saved ? '북마크 해제' : '북마크 추가'}
          aria-pressed={saved}
          className={`channel-favorite-button ${saved ? 'is-active' : ''}`}
          onClick={handleBookmark}
        >
          <StarIcon filled={saved} />
        </button>
      </Tooltip>

      <div className="channel-result-main">
        <Avatar size={42} src={channel.channelImageUrl || undefined}>
          {channel.channelName.slice(0, 1)}
        </Avatar>

        <div className="min-w-0 flex-1">
          <Space size={5} wrap>
            <Text strong ellipsis className="!max-w-44">
              {channel.channelName}
            </Text>
            {channel.verifiedMark && <Tag color="blue">인증</Tag>}
            {channel.openLive && <Tag color="green">LIVE</Tag>}
          </Space>
          <Text className="app-muted block !text-xs">
            팔로워 {followerFormatter.format(channel.followerCount)}
          </Text>
        </div>
      </div>

      {channel.channelDescription && (
        <Paragraph ellipsis={{ rows: 1 }} className="app-muted !mb-0 !mt-2 !text-xs">
          {channel.channelDescription}
        </Paragraph>
      )}
    </Card>
  );
}

export default ChannelCard;
