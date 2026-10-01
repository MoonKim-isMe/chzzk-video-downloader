import { Avatar, Button, Card, Space, Tag, Typography } from 'antd';

import type { Channel } from '../types/channel';

const { Paragraph, Text, Title } = Typography;
const followerFormatter = new Intl.NumberFormat('ko-KR');

interface ChannelCardProps {
  channel: Channel;
  saved: boolean;
  onSave: (channel: Channel) => void;
}

function ChannelCard({ channel, saved, onSave }: ChannelCardProps) {
  return (
    <Card size="small" className="border-slate-800 bg-slate-900/80">
      <div className="flex min-w-0 items-start gap-4">
        <Avatar size={56} src={channel.channelImageUrl || undefined}>
          {channel.channelName.slice(0, 1)}
        </Avatar>
        <div className="min-w-0 flex-1">
          <Space size={8} wrap>
            <Title level={5} className="!m-0 !text-slate-100">
              {channel.channelName}
            </Title>
            {channel.verifiedMark && <Tag color="blue">인증</Tag>}
            {channel.openLive && <Tag color="green">LIVE</Tag>}
          </Space>
          <Text className="block !text-slate-400">
            팔로워 {followerFormatter.format(channel.followerCount)}
          </Text>
          {channel.channelDescription && (
            <Paragraph ellipsis={{ rows: 2 }} className="!mb-0 !mt-2 !text-slate-400">
              {channel.channelDescription}
            </Paragraph>
          )}
        </div>
        <Button type={saved ? 'default' : 'primary'} disabled={saved} onClick={() => onSave(channel)}>
          {saved ? '저장됨' : '저장'}
        </Button>
      </div>
    </Card>
  );
}

export default ChannelCard;
