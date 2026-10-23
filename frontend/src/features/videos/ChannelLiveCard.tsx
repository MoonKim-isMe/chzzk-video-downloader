import { Avatar, Button, Card, Tag, Typography } from 'antd';
import { useState } from 'react';

import type { Channel } from '../../types/channel';
import type { LiveStatus } from '../../types/live';

const { Text, Title } = Typography;

interface ChannelLiveCardProps {
  channel: Channel;
  live: LiveStatus;
  onRecord: (channelId: string) => Promise<void>;
}

export default function ChannelLiveCard({ channel, live, onRecord }: ChannelLiveCardProps) {
  const [starting, setStarting] = useState(false);
  const record = async () => {
    setStarting(true);
    try {
      await onRecord(channel.channelId);
    } finally {
      setStarting(false);
    }
  };

  return (
    <Card
      size="small"
      className="video-card live-video-card h-full overflow-hidden"
      styles={{ body: { padding: 12, height: '100%' } }}
    >
      <div className="video-card-content">
        <div className="video-thumbnail live-video-thumbnail flex flex-col items-center justify-center gap-3">
          <Avatar size={64} src={channel.channelImageUrl || undefined}>
            {channel.channelName.slice(0, 1)}
          </Avatar>
          <Tag color="green" className="!m-0">LIVE · 방송 중</Tag>
        </div>
        <div className="mt-3 min-w-0">
          <Tag color="green">라이브</Tag>
          <Title level={5} ellipsis={{ rows: 2 }} className="video-card-title !mb-1 !mt-2">
            {live.liveTitle || channel.channelName + ' 라이브 방송'}
          </Title>
          <Text className="block !text-[13px]">{channel.channelName}</Text>
          <Text type="secondary" className="mt-1 block !text-[13px]">
            {live.openDate || '현재 방송 중'}
          </Text>
        </div>
        <div className="video-card-footer">
          <Text type="secondary" className="mb-2 block !text-[12px]">
            라이브는 동영상 녹화만 지원합니다.
          </Text>
          <Button type="primary" block loading={starting} onClick={() => void record()}>
            동영상 다운로드
          </Button>
        </div>
      </div>
    </Card>
  );
}
