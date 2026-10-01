import { Alert, Button, Empty, Input, Space, Typography } from 'antd';
import { useMemo, useState } from 'react';

import ChannelCard from '../../components/ChannelCard';
import { searchChannels } from '../../lib/backend';
import type { Channel, ChannelSearchResult } from '../../types/channel';

const { Paragraph, Text, Title } = Typography;

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
  const [result, setResult] = useState<ChannelSearchResult>({ channels: [], nextOffset: 0, hasNext: false });
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
    <div>
      <div className="mb-5">
        <Text className="app-eyebrow">DISCOVER</Text>
        <Title level={3} className="app-section-title !mb-1 !mt-1">
          채널 검색
        </Title>
        <Paragraph className="app-muted !mb-0">
          치지직 채널을 검색하고 원하는 채널의 VOD를 바로 확인하세요.
        </Paragraph>
      </div>

      <Input.Search
        value={keyword}
        onChange={(event) => setKeyword(event.target.value)}
        onSearch={() => void runSearch(0, false)}
        enterButton="검색"
        placeholder="채널명 또는 검색어"
        size="large"
        loading={loading}
      />

      {error && <Alert className="mt-4" type="error" showIcon message={error} />}

      {searched && !loading && (
        <div className="mb-2 mt-5 flex items-center justify-between">
          <Text strong>검색 결과</Text>
          <Text className="app-muted !text-xs">{uniqueChannels.length}개 표시</Text>
        </div>
      )}

      <Space direction="vertical" size={10} className={searched ? 'w-full' : 'mt-5 w-full'}>
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
      </Space>

      {searched && !loading && uniqueChannels.length === 0 && (
        <Empty className="py-8" description="검색 결과가 없습니다." />
      )}

      {result.hasNext && (
        <Button block className="mt-4" loading={loadingMore} onClick={() => void runSearch(result.nextOffset, true)}>
          더 보기
        </Button>
      )}
    </div>
  );
}

export default ChannelSearchTab;
