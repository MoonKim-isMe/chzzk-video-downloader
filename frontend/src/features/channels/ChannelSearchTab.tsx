import { Alert, Button, Empty, Input, Typography } from 'antd';
import { useMemo, useState } from 'react';

import ChannelCard from '../../components/ChannelCard';
import { searchChannels } from '../../lib/backend';
import type { Channel, ChannelSearchResult } from '../../types/channel';

const { Text } = Typography;

interface ChannelSearchTabProps {
  savedChannelIds: Set<string>;
  selectedChannelId?: string;
  onSelect: (channel: Channel) => void;
  onSave: (channel: Channel) => void;
}

function ChannelSearchTab({
  savedChannelIds,
  selectedChannelId,
  onSelect,
  onSave,
}: ChannelSearchTabProps) {
  const [keyword, setKeyword] = useState('');
  const [result, setResult] = useState<ChannelSearchResult>({
    channels: [],
    nextOffset: 0,
    hasNext: false,
  });
  const [searched, setSearched] = useState(false);
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState('');

  const uniqueChannels = useMemo(() => {
    const map = new Map(result.channels.map((channel) => [channel.channelId, channel]));
    return [...map.values()];
  }, [result.channels]);

  const runSearch = async (offset: number, append: boolean) => {
    const normalizedKeyword = keyword.trim();
    if (!normalizedKeyword) {
      setError('검색할 채널명을 입력해 주세요.');
      return;
    }

    append ? setLoadingMore(true) : setLoading(true);
    setError('');
    try {
      const next = await searchChannels(normalizedKeyword, offset, 20);
      setResult((current) => ({
        channels: append ? [...current.channels, ...next.channels] : next.channels,
        nextOffset: next.nextOffset,
        hasNext: next.hasNext,
      }));
      setSearched(true);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setLoading(false);
      setLoadingMore(false);
    }
  };

  return (
    <div className="channel-search-view">
      <div className="channel-search-controls">
        <Input.Search
          value={keyword}
          onChange={(event) => setKeyword(event.target.value)}
          onSearch={() => void runSearch(0, false)}
          enterButton="검색"
          placeholder="채널명 또는 검색어"
          size="large"
          loading={loading}
        />

        {error && <Alert type="error" showIcon message={error} />}

        {searched && !loading && (
          <div className="channel-result-summary">
            <Text strong>검색 결과</Text>
            <Text className="app-muted !text-xs">{uniqueChannels.length}개</Text>
          </div>
        )}
      </div>

      <div className="channel-sidebar-scroll">
        {uniqueChannels.length > 0 && (
          <div className="channel-result-list">
            {uniqueChannels.map((channel) => (
              <ChannelCard
                key={channel.channelId}
                channel={channel}
                saved={savedChannelIds.has(channel.channelId)}
                selected={selectedChannelId === channel.channelId}
                onSelect={onSelect}
                onSave={onSave}
              />
            ))}
          </div>
        )}

        {searched && !loading && uniqueChannels.length === 0 && (
          <Empty className="channel-sidebar-empty" description="검색 결과가 없습니다." />
        )}

        {result.hasNext && (
          <div className="channel-load-more">
            <Button
              block
              loading={loadingMore}
              onClick={() => void runSearch(result.nextOffset, true)}
            >
              더 보기
            </Button>
          </div>
        )}
      </div>
    </div>
  );
}

export default ChannelSearchTab;
