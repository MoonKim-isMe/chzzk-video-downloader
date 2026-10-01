import { Avatar, Button, Card, Space, Tag, Typography } from 'antd';

import type { Channel } from '../types/channel';

const { Paragraph, Text } = Typography;
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
      styles={{ body: { padding: 12 } }}
    >
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

      <div className="channel-result-actions">
        <Button size="small" type={selected ? 'primary' : 'default'} onClick={() => onSelect(channel)}>
          {selected ? '선택됨' : '선택'}
        </Button>
        <Button size="small" disabled={saved} onClick={() => onSave(channel)}>
          {saved ? '북마크됨' : '북마크'}
        </Button>
      </div>
    </Card>
  );
}

export default ChannelCard;
