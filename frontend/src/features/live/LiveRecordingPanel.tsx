import { Alert, App as AntdApp, Button, Card, Checkbox, Empty, Input, Select, Space, Tag, Typography } from 'antd';
import { useCallback, useEffect, useState } from 'react';

import { extractLiveClip, getLiveRecordingState, startLiveRecording, stopLiveRecording } from '../../lib/backend';
import type { LiveSession, LiveState } from '../../types/live';

const { Text, Title } = Typography;

const initialState: LiveState = {
  pollSeconds: 30,
  maxConcurrent: 2,
  subscriptions: [],
  sessions: [],
  clips: [],
};

function toSeconds(value: string): number | undefined {
  const match = /^(?:(\d+):)?([0-5]?\d):([0-5]\d)$/.exec(value.trim());
  if (!match) return undefined;
  return Number(match[1] ?? 0) * 3600 + Number(match[2]) * 60 + Number(match[3]);
}

function formatTime(value: number) {
  const seconds = Math.max(0, Math.floor(value));
  return [Math.floor(seconds / 3600), Math.floor(seconds / 60) % 60, seconds % 60]
    .map((part) => String(part).padStart(2, '0')).join(':');
}

function statusLabel(status: LiveSession['status']) {
  return ({
    recording: '녹화 중',
    finalizing: '파일 생성 중',
    completed: '녹화 완료',
    failed: '실패',
    interrupted: '녹화 중단',
  })[status];
}

function ClipEditor({ session, onUpdated }: { session: LiveSession; onUpdated: () => void }) {
  const { message } = AntdApp.useApp();
  const [title, setTitle] = useState('하이라이트');
  const [start, setStart] = useState('00:00:00');
  const [end, setEnd] = useState('00:01:00');
  const [exact, setExact] = useState(false);
  const [working, setWorking] = useState(false);

  const extract = async () => {
    const a = toSeconds(start);
    const b = toSeconds(end);
    if (a === undefined || b === undefined || b <= a || b > session.availableSeconds) {
      message.warning('이미 저장된 구간 안에서 올바른 시작·종료 시간을 입력해 주세요.');
      return;
    }
    setWorking(true);
    try {
      await extractLiveClip(session.id, title.trim() || '하이라이트', a, b, exact);
      message.success('구간 추출을 시작했습니다. 라이브 녹화는 계속됩니다.');
      onUpdated();
    } catch (cause) {
      message.error(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setWorking(false);
    }
  };

  return (
    <div className="mt-5 rounded-xl border border-solid border-slate-500/25 p-4">
      <Text strong className="mb-1 block">녹화된 구간 추출</Text>
      <Text type="secondary" className="mb-4 block text-xs">
        저장이 끝난 구간만 추출할 수 있습니다. 원본 녹화는 중단되지 않습니다.
      </Text>
      <div className="grid gap-3 sm:grid-cols-[minmax(0,1fr)_auto_auto] sm:items-end">
        <div>
          <Text className="mb-1 block text-xs">파일 제목</Text>
          <Input value={title} onChange={(event) => setTitle(event.target.value)} aria-label="추출 파일 제목" />
        </div>
        <div>
          <Text className="mb-1 block text-xs">시작 시간</Text>
          <Input value={start} onChange={(event) => setStart(event.target.value)} aria-label="시작 시간" placeholder="00:00:00" style={{ width: 110 }} />
        </div>
        <div>
          <Text className="mb-1 block text-xs">종료 시간</Text>
          <Input value={end} onChange={(event) => setEnd(event.target.value)} aria-label="종료 시간" placeholder="00:01:00" style={{ width: 110 }} />
        </div>
      </div>
      <div className="mt-3 flex flex-wrap items-center justify-between gap-3">
        <Checkbox checked={exact} onChange={(event) => setExact(event.target.checked)}>
          정확한 시간 경계 (재인코딩)
        </Checkbox>
        <Button type="primary" loading={working} disabled={session.availableSeconds <= 0} onClick={() => void extract()}>
          MP4 추출
        </Button>
      </div>
      <Text type="secondary" className="mt-3 block text-xs">
        추출 가능: 00:00:00 ~ {formatTime(session.availableSeconds)}
      </Text>
    </div>
  );
}

export default function LiveRecordingPanel() {
  const { message } = AntdApp.useApp();
  const [state, setState] = useState<LiveState>(initialState);
  const [error, setError] = useState('');
  const [selectedKey, setSelectedKey] = useState<string>();
  const [busy, setBusy] = useState(false);

  const refresh = useCallback(async () => {
    const next = await getLiveRecordingState();
    setState(next);
    setError('');
  }, []);

  useEffect(() => {
    let active = true;
    const read = async () => {
      try {
        const next = await getLiveRecordingState();
        if (active) {
          setState(next);
          setError('');
        }
      } catch (cause) {
        if (active) setError(cause instanceof Error ? cause.message : String(cause));
      }
    };
    void read();
    const timer = window.setInterval(() => { void read(); }, 3000);
    return () => { active = false; window.clearInterval(timer); };
  }, []);

  const perform = async (action: () => Promise<unknown>, successMessage?: string) => {
    if (busy) return;
    setBusy(true);
    try {
      await action();
      await refresh();
      if (successMessage) message.success(successMessage);
    } catch (cause) {
      message.error(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setBusy(false);
    }
  };

  const recording = state.sessions.filter((session) => session.status === 'recording');
  const finalizing = state.sessions.filter((session) => session.status === 'finalizing');
  const activeSessions = [...recording, ...finalizing];
  const watchedChannels = state.subscriptions.filter((subscription) => subscription.enabled);

  const requestedSession = selectedKey?.startsWith('session:')
    ? state.sessions.find((session) => session.id === selectedKey.slice('session:'.length))
    : undefined;
  const requestedChannelId = selectedKey?.startsWith('channel:')
    ? selectedKey.slice('channel:'.length)
    : undefined;
  const channelSession = requestedChannelId
    ? activeSessions.find((session) => session.channelId === requestedChannelId)
    : undefined;
  const selectedSession = requestedSession ?? channelSession ?? (!selectedKey ? activeSessions[0] ?? state.sessions[0] : undefined);
  const selectedChannel = requestedChannelId
    ? state.subscriptions.find((subscription) => subscription.channelId === requestedChannelId)
    : undefined;
  const selectedSessionClips = state.clips.filter((clip) => clip.sessionId === selectedSession?.id);

  const selectSession = (id: string) => setSelectedKey('session:' + id);
  const recordChannel = (id: string) => void perform(async () => {
    const session = await startLiveRecording(id);
    selectSession(session.id);
  }, '라이브 동영상 녹화를 시작했습니다.');

  return (
    <div className="live-recording-workspace">
      <Card bordered={false} className="app-panel live-recording-sidebar">
        <div className="live-recording-sidebar-heading">
          <Title level={5} className="!mb-1">라이브 채널</Title>
          <Text type="secondary" className="text-xs">자동 감시 및 현재 녹화 상태</Text>
        </div>
        <div className="live-recording-sidebar-scroll">
          <div className="live-recording-summary">
            <div className="flex items-center justify-between">
              <Text strong>현재 설정</Text>
              <Tag color="green">VOD와 별도</Tag>
            </div>
            <div className="mt-2 flex justify-between gap-2">
              <Text type="secondary" className="text-xs">방송 확인 간격</Text>
              <Text strong>{state.pollSeconds}초</Text>
            </div>
            <div className="mt-1 flex justify-between gap-2">
              <Text type="secondary" className="text-xs">라이브 동시 녹화</Text>
              <Text strong>{recording.length} / {state.maxConcurrent}개</Text>
            </div>
            <Text type="secondary" className="mt-2 block text-xs">
              설정 변경은 우측 상단 설정에서 가능합니다.
            </Text>
          </div>

          <div className="live-recording-list-heading">
            <Text strong>자동 녹화 채널</Text>
            <Tag>{watchedChannels.length}개</Tag>
          </div>
          {watchedChannels.length === 0 ? (
            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="등록된 자동 녹화 채널이 없습니다." />
          ) : (
            <div className="flex flex-col gap-2">
              {watchedChannels.map((channel) => {
                const isActive = activeSessions.some((session) => session.channelId === channel.channelId);
                const selected = selectedKey === 'channel:' + channel.channelId;
                return (
                  <button
                    key={channel.channelId}
                    type="button"
                    aria-pressed={selected}
                    className={'live-recording-list-item' + (selected ? ' is-selected' : '')}
                    onClick={() => setSelectedKey('channel:' + channel.channelId)}
                  >
                    <span className="min-w-0 flex-1 text-left">
                      <Text strong ellipsis className="block">{channel.channelName || channel.channelId}</Text>
                      <Text type="secondary" className="block truncate text-xs">
                        {channel.lastError || (isActive ? '녹화 진행 중' : '방송 감시 중')}
                      </Text>
                    </span>
                    <Tag color={isActive ? 'green' : undefined} className="!mr-0">
                      {isActive ? '녹화 중' : '자동'}
                    </Tag>
                  </button>
                );
              })}
            </div>
          )}

          <div className="live-recording-list-heading">
            <Text strong>현재 녹화 중</Text>
            <Tag color={activeSessions.length ? 'green' : undefined}>{activeSessions.length}개</Tag>
          </div>
          {activeSessions.length === 0 ? (
            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="현재 녹화 중인 채널이 없습니다." />
          ) : (
            <div className="flex flex-col gap-2">
              {activeSessions.map((session) => {
                const selected = selectedKey === 'session:' + session.id || (!selectedKey && selectedSession?.id === session.id);
                return (
                  <button
                    key={session.id}
                    type="button"
                    aria-pressed={selected}
                    className={'live-recording-list-item' + (selected ? ' is-selected' : '')}
                    onClick={() => selectSession(session.id)}
                  >
                    <span className="min-w-0 flex-1 text-left">
                      <Text strong ellipsis className="block">{session.channelName || session.channelId}</Text>
                      <Text type="secondary" className="block truncate text-xs">
                        {session.title || '라이브 방송'} · {formatTime(session.availableSeconds)}
                      </Text>
                    </span>
                    <Tag color="processing" className="!mr-0">{session.status === 'finalizing' ? '저장 중' : '녹화 중'}</Tag>
                  </button>
                );
              })}
            </div>
          )}
        </div>
      </Card>

      <Card bordered={false} className="app-panel live-recording-detail">
        <div className="live-recording-detail-heading">
          <div className="min-w-0">
            <Title level={4} className="!mb-1">라이브 녹화</Title>
            <Text type="secondary" className="text-xs">녹화 관리와 완료된 구간의 MP4 추출</Text>
          </div>
          <Tag color="green">라이브 {activeSessions.length} / {state.maxConcurrent}</Tag>
        </div>
        <div className="live-recording-detail-scroll">
          {error && <Alert type="warning" showIcon message="라이브 녹화 상태를 불러올 수 없습니다." description={error} className="!mb-4" />}
          {state.sessions.length > 0 && (
            <div className="mb-4">
              <Text type="secondary" className="mb-2 block text-xs">녹화 기록 선택</Text>
              <Select
                aria-label="녹화 기록 선택"
                className="w-full"
                value={selectedSession?.id}
                placeholder="녹화 기록을 선택하세요"
                showSearch
                optionFilterProp="label"
                onChange={selectSession}
                options={state.sessions.slice(0, 100).map((session) => ({
                  value: session.id,
                  label: (session.channelName || session.channelId) + ' · ' + (session.title || '라이브') + ' (' + statusLabel(session.status) + ')',
                }))}
              />
            </div>
          )}

          {!selectedSession && selectedChannel ? (
            <div className="rounded-xl border border-solid border-slate-500/25 p-5">
              <Title level={5} className="!mb-1">{selectedChannel.channelName || selectedChannel.channelId}</Title>
              <Text type="secondary" className="mb-4 block text-sm">
                자동 녹화 채널로 등록되어 있습니다. 방송 시작을 감지하면 자동으로 녹화합니다.
              </Text>
              {selectedChannel.lastError && <Alert type="warning" showIcon message={selectedChannel.lastError} className="!mb-3" />}
              <Button type="primary" loading={busy} onClick={() => recordChannel(selectedChannel.channelId)}>
                지금 동영상 녹화
              </Button>
            </div>
          ) : !selectedSession ? (
            <Empty description="왼쪽에서 채널을 선택하거나 라이브 녹화를 시작해 주세요." />
          ) : (
            <div className="flex flex-col gap-4">
              <div className="rounded-xl border border-solid border-slate-500/25 p-5">
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div className="min-w-0 flex-1">
                    <Space wrap size={8}>
                      <Tag color={selectedSession.status === 'recording' ? 'green' : undefined}>
                        {statusLabel(selectedSession.status)}
                      </Tag>
                      <Text type="secondary" className="text-xs">{selectedSession.channelName}</Text>
                    </Space>
                    <Title level={5} className="!mb-2 !mt-3">{selectedSession.title || '라이브 방송'}</Title>
                    <Text type="secondary" className="block text-xs">
                      저장된 구간 {formatTime(selectedSession.availableSeconds)}
                    </Text>
                    <Text type="secondary" className="mt-1 block break-all text-xs">
                      {selectedSession.finalPath || selectedSession.directory}
                    </Text>
                  </div>
                  {selectedSession.status === 'recording' && (
                    <Button
                      danger
                      loading={busy}
                      onClick={() => void perform(() => stopLiveRecording(selectedSession.id), '녹화 중지를 요청했습니다.')}
                    >
                      녹화 중지
                    </Button>
                  )}
                </div>
                {selectedSession.error && <Alert type="warning" showIcon message={selectedSession.error} className="!mt-4" />}
                <ClipEditor key={selectedSession.id} session={selectedSession} onUpdated={() => void refresh()} />
              </div>

              <div>
                <Text strong className="mb-3 block">추출 내역</Text>
                {selectedSessionClips.length === 0 ? (
                  <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="추출한 구간이 없습니다." />
                ) : (
                  <div className="flex flex-col gap-2">
                    {selectedSessionClips.map((clip) => (
                      <div key={clip.id} className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-solid border-slate-500/25 p-3">
                        <div className="min-w-0">
                          <Text strong className="block">{clip.title}</Text>
                          <Text type="secondary" className="block text-xs">
                            {formatTime(clip.startSeconds)} ~ {formatTime(clip.endSeconds)}
                          </Text>
                          {(clip.filePath || clip.error) && (
                            <Text type={clip.error ? 'danger' : 'secondary'} className="block break-all text-xs">
                              {clip.filePath || clip.error}
                            </Text>
                          )}
                        </div>
                        <Tag color={clip.status === 'completed' ? 'success' : clip.status === 'failed' ? 'error' : 'processing'}>
                          {({ queued: '대기 중', running: '추출 중', completed: '완료', failed: '실패' })[clip.status]}
                        </Tag>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </div>
          )}
        </div>
      </Card>
    </div>
  );
}
