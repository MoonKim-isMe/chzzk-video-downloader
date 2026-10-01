import { Avatar, Button, Empty, List, Tag, Typography } from 'antd';

import type { Channel } from '../types/channel';

const { Text } = Typography;

interface SavedChannelsProps {
  channels: Channel[];
  selectedChannelId?: string;
  onSelect: (channel: Channel) => void;
  onRemove: (channelId: string) => void;
}

function SavedChannels({ channels, selectedChannelId, onSelect, onRemove }: SavedChannelsProps) {
  return (
    <div className="channel-bookmarks-view">
      <div className="channel-sidebar-summary">
        <Text strong>북마크 채널</Text>
        <Text className="app-muted !text-xs">{channels.length}개</Text>
      </div>

      <div className="channel-sidebar-scroll">
        {channels.length === 0 ? (
          <Empty
            className="channel-sidebar-empty"
            image={Empty.PRESENTED_IMAGE_SIMPLE}
            description="북마크한 채널이 없습니다."
          />
        ) : (
          <List
            className="bookmark-list"
            dataSource={channels}
            split={false}
            renderItem={(channel) => {
              const selected = selectedChannelId === channel.channelId;
              return (
                <List.Item className="!border-0 !p-0">
                  <div className={`bookmark-item ${selected ? 'is-selected' : ''}`}>
                    <button
                      type="button"
                      className="bookmark-channel-main"
                      onClick={() => onSelect(channel)}
                    >
                      <Avatar size={40} src={channel.channelImageUrl || undefined}>
                        {channel.channelName.slice(0, 1)}
                      </Avatar>
                      <span className="min-w-0 flex-1 text-left">
                        <Text ellipsis className="block !font-medium">
                          {channel.channelName}
                        </Text>
                        <Text className="app-muted block !text-xs">
                          {selected ? '선택된 채널' : 'VOD 보기'}
                        </Text>
                      </span>
                      {selected && <Tag color="green">선택됨</Tag>}
                    </button>
                    <Button
                      type="text"
                      danger
                      size="small"
                      className="bookmark-remove"
                      onClick={() => onRemove(channel.channelId)}
                    >
                      삭제
                    </Button>
                  </div>
                </List.Item>
              );
            }}
          />
        )}
      </div>
    </div>
  );
}

export default SavedChannels;
