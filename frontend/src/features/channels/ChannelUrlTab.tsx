import { Alert, Empty, Input, Typography } from 'antd';
import { useState } from 'react';

import ChannelCard from '../../components/ChannelCard';
import { resolveChannelURL } from '../../lib/backend';
import type { Channel } from '../../types/channel';

const { Paragraph, Title } = Typography;

interface ChannelUrlTabProps {
  savedChannelIds: Set<string>;
  onSave: (channel: Channel) => void;
}

function ChannelUrlTab({ savedChannelIds, onSave }: ChannelUrlTabProps) {
  const [url, setURL] = useState('');
  const [channel, setChannel] = useState<Channel>();
  const [loading, setLoading] = useState(false);
  const [resolved, setResolved] = useState(false);
  const [error, setError] = useState('');

  const resolve = async () => {
    setLoading(true);
    setError('');
    setChannel(undefined);
    try {
      const nextChannel = await resolveChannelURL(url);
      setChannel(nextChannel);
      setResolved(true);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
      setResolved(true);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div>
      <Title level={3} className="!mb-2 !text-slate-100">
        URL 직접 입력
      </Title>
      <Paragraph className="!text-slate-400">
        치지직 채널 URL에서 channelId를 추출한 뒤 실제 채널 정보를 확인합니다.
      </Paragraph>

      <Input.Search
        value={url}
        onChange={(event) => setURL(event.target.value)}
        onSearch={resolve}
        enterButton="채널 확인"
        placeholder="https://chzzk.naver.com/{channelId}"
        size="large"
        loading={loading}
      />

      {error && <Alert className="mt-4" type="error" showIcon message={error} />}
      {channel && (
        <div className="mt-5">
          <ChannelCard
            channel={channel}
            saved={savedChannelIds.has(channel.channelId)}
            onSave={onSave}
          />
        </div>
      )}
      {resolved && !loading && !channel && !error && (
        <Empty className="mt-12" description="채널 정보를 찾을 수 없습니다." />
      )}
    </div>
  );
}

export default ChannelUrlTab;
