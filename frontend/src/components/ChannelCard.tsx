import { Avatar, Button, Card, Space, Tag, Typography } from 'antd';

import type { Channel } from '../types/channel';

const { Paragraph, Text, Title } = Typography;
const followerFormatter = new Intl.NumberFormat('ko-KR');

interface ChannelCardProps {
  channel: Channel;
  saved: boolean;
  selected?: boolean;
  onSelect: (channel: Channel) => void;
  onSave: (channel: Channel) => void;
}

function ChannelCard({ channel, saved, selected = false, onSelect, onSave }: ChannelCardProps) {
  return (
    <Card
      size="small"
      bordered={false}
      className={`channel-result-card ${selected ? 'is-selected' : ''}`}
    >
      <div className="flex min-w-0 items-center gap-3">
        <Avatar size={48} src={channel.channelImageUrl || undefined}>
          {channel.channelName.slice(0, 1)}
        </Avatar>

        <div className="min-w-0 flex-1">
          <Space size={6} wrap>
            <Title level={5} className="!m-0">
              {channel.channelName}
            </Title>
            {channel.verifiedMark && <Tag color="blue">인증</Tag>}
            {channel.openLive && <Tag color="green">LIVE</Tag>}
          </Space>
          <Text className="app-muted block !text-xs">
            팔로워 {followerFormatter.format(channel.followerCount)}
          </Text>
          {channel.channelDescription && (
            <Paragraph ellipsis={{ rows: 1 }} className="app-muted !mb-0 !mt-1 !text-xs">
              {channel.channelDescription}
            </Paragraph>
          )}
        </div>

        <Space size={6}>
          <Button type={selected ? 'primary' : 'default'} onClick={() => onSelect(channel)}>
            {selected ? '선택됨' : '선택'}
          </Button>
          <Button disabled={saved} onClick={() => onSave(channel)}>
            {saved ? '북마크됨' : '북마크'}
          </Button>
        </Space>
      </div>
    </Card>
  );
}

export default ChannelCard;
