import { Button, Empty, Input, InputNumber, Select, Space, Switch, Tag, Typography } from 'antd';
import { useState } from 'react';

import type { Channel } from '../../types/channel';
import type { LiveState, LiveSubscription } from '../../types/live';

const { Text } = Typography;

interface LiveAutoRecordingSettingsProps {
  value: LiveState;
  onChange: (next: LiveState) => void;
  savedChannels: Channel[];
}

function parseChannelId(raw: string): string | undefined {
  const input = raw.trim();
  if (/^[a-f0-9]{32}$/i.test(input)) return input.toLowerCase();
  const match = /^https:\/\/chzzk\.naver\.com\/(?:live\/)?([a-f0-9]{32})\/?(?:[?#].*)?$/i.exec(input);
  return match?.[1].toLowerCase();
}

export default function LiveAutoRecordingSettings({
  value,
  onChange,
  savedChannels,
}: LiveAutoRecordingSettingsProps) {
  const [channelInput, setChannelInput] = useState('');
  const [inputError, setInputError] = useState('');
  const watchedIds = new Set(value.subscriptions.map((channel) => channel.channelId));
  const channelOptions = savedChannels
    .filter((channel) => !watchedIds.has(channel.channelId))
    .map((channel) => ({ value: channel.channelId, label: channel.channelName }));

  const addChannel = (channelId: string, channelName = '') => {
    if (watchedIds.has(channelId)) {
      setInputError('이미 등록된 채널입니다.');
      return;
    }
    const next: LiveSubscription = { channelId, channelName, enabled: true };
    onChange({ ...value, subscriptions: [...value.subscriptions, next] });
    setInputError('');
    setChannelInput('');
  };

  const addFromInput = () => {
    const id = parseChannelId(channelInput);
    if (!id) {
      setInputError('올바른 치지직 채널 ID 또는 URL을 입력해 주세요.');
      return;
    }
    const saved = savedChannels.find((channel) => channel.channelId === id);
    addChannel(id, saved?.channelName);
  };

  return (
    <div className="flex flex-col gap-4">
      <div className="grid grid-cols-2 gap-3">
        <div>
          <Text className="mb-2 block text-sm">방송 확인 간격</Text>
          <Select
            aria-label="라이브 방송 확인 간격"
            value={value.pollSeconds}
            className="w-full"
            options={[15, 30, 60].map((seconds) => ({ label: seconds + '초', value: seconds }))}
            onChange={(pollSeconds) => onChange({ ...value, pollSeconds })}
          />
        </div>
        <div>
          <Text className="mb-2 block text-sm">동시 라이브 녹화 수</Text>
          <InputNumber
            aria-label="동시 라이브 녹화 수"
            min={1}
            max={4}
            precision={0}
            value={value.maxConcurrent}
            className="!w-full"
            onChange={(maxConcurrent) => {
              if (maxConcurrent !== null) onChange({ ...value, maxConcurrent });
            }}
          />
        </div>
      </div>

      <Text type="secondary" className="text-xs">
        VOD 다운로드 수와 별도로 적용됩니다. 자동 감시는 앱이 실행 중일 때만 작동합니다.
      </Text>

      <div className="flex flex-col gap-2">
        <div className="flex items-center justify-between">
          <Text strong>자동 녹화 채널</Text>
          <Tag>{value.subscriptions.filter((channel) => channel.enabled).length}개 활성</Tag>
        </div>
        <Select
          className="w-full"
          value={undefined}
          placeholder="북마크에서 채널 추가"
          showSearch
          optionFilterProp="label"
          options={channelOptions}
          disabled={channelOptions.length === 0}
          onChange={(id: string) => {
            const channel = savedChannels.find((saved) => saved.channelId === id);
            if (channel) addChannel(id, channel.channelName);
          }}
        />
        <Space.Compact className="w-full">
          <Input
            aria-label="자동 녹화 채널 URL"
            placeholder="채널 URL 또는 채널 ID"
            value={channelInput}
            onChange={(event) => {
              setChannelInput(event.target.value);
              if (inputError) setInputError('');
            }}
            onPressEnter={addFromInput}
          />
          <Button onClick={addFromInput} disabled={!channelInput.trim()}>추가</Button>
        </Space.Compact>
        {inputError && <Text type="danger" className="text-xs">{inputError}</Text>}
      </div>

      <div className="flex max-h-60 flex-col gap-2 overflow-y-auto">
        {value.subscriptions.length === 0 && (
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="자동 녹화 채널이 없습니다." />
        )}
        {value.subscriptions.map((subscription) => {
          const saved = savedChannels.find((channel) => channel.channelId === subscription.channelId);
          return (
            <div
              key={subscription.channelId}
              className="flex items-center gap-2 rounded-lg border border-solid border-slate-500/25 p-2.5"
            >
              <div className="min-w-0 flex-1">
                <Text strong ellipsis className="block">
                  {subscription.channelName || saved?.channelName || subscription.channelId}
                </Text>
                <Text type="secondary" className="block truncate text-xs">
                  {subscription.channelId}
                </Text>
              </div>
              <Switch
                size="small"
                aria-label="자동 녹화 사용"
                checked={subscription.enabled}
                onChange={(enabled) => onChange({
                  ...value,
                  subscriptions: value.subscriptions.map((entry) => entry.channelId === subscription.channelId
                    ? { ...entry, enabled }
                    : entry),
                })}
              />
              <Button
                size="small"
                type="text"
                danger
                onClick={() => onChange({
                  ...value,
                  subscriptions: value.subscriptions.filter((entry) => entry.channelId !== subscription.channelId),
                })}
              >
                삭제
              </Button>
            </div>
          );
        })}
      </div>
      <Text type="secondary" className="text-xs">변경 사항은 아래 저장 버튼을 누르면 적용됩니다.</Text>
    </div>
  );
}
