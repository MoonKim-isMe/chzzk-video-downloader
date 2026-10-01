import { Avatar, Button, Empty, List, Space, Tag, Typography } from 'antd';

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
    <div className="rounded-xl border border-slate-800 bg-slate-900/70 p-4">
      <div className="mb-3">
        <Text strong className="!text-slate-100">
          저장 채널
        </Text>
        <Text className="ml-2 !text-slate-500">{channels.length}</Text>
      </div>

      {channels.length === 0 ? (
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="저장된 채널이 없습니다." />
      ) : (
        <List
          dataSource={channels}
          split={false}
          renderItem={(channel) => {
            const selected = selectedChannelId === channel.channelId;
            return (
              <List.Item className="!px-0 !py-2">
                <div className="w-full rounded-lg border border-slate-800 bg-slate-950/50 p-3">
                  <div className="flex items-center gap-3">
                    <Avatar src={channel.channelImageUrl || undefined}>
                      {channel.channelName.slice(0, 1)}
                    </Avatar>
                    <div className="min-w-0 flex-1">
                      <Space size={6}>
                        <Text ellipsis className="!max-w-36 !text-slate-200">
                          {channel.channelName}
                        </Text>
                        {selected && <Tag color="green">선택됨</Tag>}
                      </Space>
                    </div>
                  </div>
                  <Space className="mt-3 w-full justify-end">
                    <Button size="small" type={selected ? 'primary' : 'default'} onClick={() => onSelect(channel)}>
                      선택
                    </Button>
                    <Button size="small" danger onClick={() => onRemove(channel.channelId)}>
                      삭제
                    </Button>
                  </Space>
                </div>
              </List.Item>
            );
          }}
        />
      )}
    </div>
  );
}

export default SavedChannels;
