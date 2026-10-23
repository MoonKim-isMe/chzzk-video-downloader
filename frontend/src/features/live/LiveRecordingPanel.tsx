import { Alert, App as AntdApp, Avatar, Button, Card, Checkbox, Empty, Input, Space, Switch, Tag, Typography } from 'antd';
import { useCallback, useEffect, useRef, useState } from 'react';

import { extractLiveClip, getChannelLiveStatus, getLiveRecordingState, setLiveAutoRecording, startLiveRecording, stopLiveRecording } from '../../lib/backend';
import type { LiveSession, LiveState, LiveStatus } from '../../types/live';
import type { Channel } from '../../types/channel';
import { formatBroadcastOffset, formatChzzkDateTime } from '../../lib/liveRecordingTime';

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

type BroadcastState = 'live' | 'offline' | 'unavailable';

interface BroadcastSnapshot {
  channelId: string;
  status: BroadcastState;
  live?: LiveStatus;
}

function useChannelBroadcastStatus(channelId: string): BroadcastSnapshot | undefined {
  const [snapshot, setSnapshot] = useState<BroadcastSnapshot>();

  useEffect(() => {
    let active = true;
    let latestRequest = 0;
    setSnapshot(undefined);

    const check = async () => {
      const request = ++latestRequest;
      try {
        const live = await getChannelLiveStatus(channelId);
        if (active && request === latestRequest) {
          setSnapshot({ channelId, status: live.isLive ? 'live' : 'offline', live: live.isLive ? live : undefined });
        }
      } catch {
        if (active && request === latestRequest) {
          setSnapshot({ channelId, status: 'unavailable' });
        }
      }
    };

    void check();
    const timer = window.setInterval(() => { void check(); }, 30_000);
    return () => {
      active = false;
      window.clearInterval(timer);
    };
  }, [channelId]);

  return snapshot?.channelId === channelId ? snapshot.status : undefined;
}

function BroadcastStatusTag({ status }: { status: BroadcastState | undefined }) {
  if (!status) return null;
  const labels: Record<BroadcastState, string> = {
    live: '방송 중',
    offline: '방송 중 아님',
    unavailable: '상태 확인 실패',
  };
  return (
    <Tag color={status === 'live' ? 'green' : undefined} className="!m-0">
      {labels[status]}
    </Tag>
  );
}

function ChannelBroadcastStatus({ channelId }: { channelId: string }) {
  const snapshot = useChannelBroadcastStatus(channelId);
  return <BroadcastStatusTag status={snapshot?.status} />;
}

interface ChannelRecordingSetupProps {
  channel: Channel;
  reserved: boolean;
  subscriptionError?: string;
  reservationReady: boolean;
  reservationBusy: boolean;
  recordingBusy: boolean;
  onReservationChange: (enabled: boolean) => void;
  onRecord: () => void;
}

function ChannelRecordingSetup({
  channel,
  reserved,
  subscriptionError,
  reservationReady,
  reservationBusy,
  recordingBusy,
  onReservationChange,
  onRecord,
}: ChannelRecordingSetupProps) {
  const snapshot = useChannelBroadcastStatus(channel.channelId);
  const status = snapshot?.status;
  return (
    <div className="rounded-xl border border-solid border-slate-500/25 p-5">
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <Title level={5} className="!m-0">{channel.channelName || channel.channelId}</Title>
        <BroadcastStatusTag status={status} />
      </div>
      {status === 'live' && snapshot?.live && (
        <>
          <LiveBroadcastPreview live={snapshot.live} />
          <Text type="secondary" className="mb-4 block text-sm">
            방송 중입니다. 지금 녹화를 시작할 수 있습니다.
          </Text>
        </>
      )}
      {status === 'offline' && (
        <Text type="secondary" className="mb-4 block text-sm">
          방송중이 아닙니다. 방송 중일때 다시 시도해주세요.
        </Text>
      )}
      {status === 'unavailable' && (
        <Text type="secondary" className="mb-4 block text-sm">
          방송 상태를 확인할 수 없습니다. 잠시 후 다시 시도해 주세요.
        </Text>
      )}
      {subscriptionError && <Alert type="warning" showIcon message={subscriptionError} className="!mb-3" />}
      <div className="flex flex-wrap items-center justify-between gap-4">
        <Button type="primary" loading={recordingBusy} disabled={status !== 'live'} onClick={onRecord}>
          지금 동영상 녹화
        </Button>
        <div className="flex items-center gap-2">
          <Text>라이브 예약</Text>
          <Switch
            aria-label="라이브 예약"
            checked={reserved}
            loading={reservationBusy}
            disabled={!reservationReady || reservationBusy}
            onChange={onReservationChange}
          />
        </div>
      </div>
      {reserved && (
        <Text type="secondary" className="mt-3 block text-xs">
          예약 중입니다. 방송 시작을 감지하면 자동으로 녹화합니다.
        </Text>
      )}
    </div>
  );
}

function LiveBroadcastPreview({ live }: { live: LiveStatus }) {
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    setNow(Date.now());
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [live.liveId, live.openDate]);

  const streamingTime = formatBroadcastOffset(new Date(now).toISOString(), live.openDate);
  const tags = [...new Set(live.tags ?? [])].map((tag) => tag.trim()).filter(Boolean);

  return (
    <div className="live-recording-session-header mb-4">
      <RecordingThumbnail url={live.thumbnailUrl} title={live.liveTitle || '라이브'} />
      <div className="min-w-0">
        <Text type="secondary" className="mb-1 block text-xs">방송 제목</Text>
        <Title level={5} className="!mb-3 !mt-0 break-words">
          {live.liveTitle || '제목 없는 방송'}
        </Title>
        <div className="mb-2">
          <Text type="secondary" className="mr-2 text-xs">방송 시작 시간</Text>
          <Text className="text-sm">{formatChzzkDateTime(live.openDate)}</Text>
        </div>
        <div className="mb-3">
          <Text type="secondary" className="mr-2 text-xs">스트리밍 시간</Text>
          <Text className="text-sm tabular-nums" strong>{streamingTime ?? '시간 정보 없음'}</Text>
        </div>
        <Text type="secondary" className="mb-2 block text-xs">태그</Text>
        <div className="flex flex-wrap gap-1.5">
          {live.liveCategoryValue && (
            <Tag color="green" className="!m-0">{live.liveCategoryValue}</Tag>
          )}
          {tags.map((tag) => (
            <Tag key={tag} className="!m-0">#{tag.replace(/^#/, '')}</Tag>
          ))}
          {!live.liveCategoryValue && tags.length === 0 && (
            <Text type="secondary" className="text-xs">등록된 태그가 없습니다.</Text>
          )}
        </div>
      </div>
    </div>
  );
}

function RecordingThumbnail({ url, title }: { url?: string; title: string }) {
  const [failed, setFailed] = useState(false);
  useEffect(() => setFailed(false), [url]);
  return (
    <div className="live-recording-session-thumbnail">
      {url && !failed ? (
        <img
          src={url}
          alt={title + ' 썸네일'}
          loading="lazy"
          className="h-full w-full object-cover"
          onError={() => setFailed(true)}
        />
      ) : (
        <div className="flex h-full w-full items-center justify-center">
          <Text type="secondary" className="text-xs">썸네일 없음</Text>
        </div>
      )}
    </div>
  );
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

export default function LiveRecordingPanel({ savedChannels }: { savedChannels: Channel[] }) {
  const { message } = AntdApp.useApp();
  const [state, setState] = useState<LiveState>(initialState);
  const [error, setError] = useState('');
  const [selectedKey, setSelectedKey] = useState<string>();
  const [busy, setBusy] = useState(false);
  const [reservationBusy, setReservationBusy] = useState(false);
  const [reservationReady, setReservationReady] = useState(false);
  const seenSessionIds = useRef<Set<string> | null>(null);

  const refresh = useCallback(async () => {
    const next = await getLiveRecordingState();
    setState(next);
    setReservationReady(true);
    setError('');
  }, []);

  useEffect(() => {
    let active = true;
    const read = async () => {
      try {
        const next = await getLiveRecordingState();
        if (active) {
          // Never open an old completed session just because it is stored in history.
          // Automatically focus an active session on first load, or a newly started
          // recording when no channel/session has been selected by the user.
          const previousIds = seenSessionIds.current;
          const newRecording = next.sessions.find((session) =>
            session.status === 'recording' &&
            (previousIds === null || !previousIds.has(session.id)),
          );
          seenSessionIds.current = new Set(next.sessions.map((session) => session.id));
          if (newRecording) {
            setSelectedKey((current) => current ?? 'session:' + newRecording.id);
          }
          setState(next);
          setReservationReady(true);
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
  const subscriptions = new Map(state.subscriptions.map((subscription) => [subscription.channelId, subscription]));

  const requestedSession = selectedKey?.startsWith('session:')
    ? state.sessions.find((session) => session.id === selectedKey.slice('session:'.length))
    : undefined;
  const requestedChannelId = selectedKey?.startsWith('channel:')
    ? selectedKey.slice('channel:'.length)
    : undefined;
  const channelSession = requestedChannelId
    ? activeSessions.find((session) => session.channelId === requestedChannelId)
    : undefined;
  const selectedSession = requestedSession ?? channelSession;
  const selectedChannel = requestedChannelId
    ? savedChannels.find((channel) => channel.channelId === requestedChannelId)
    : undefined;
  const selectedSubscription = selectedChannel ? subscriptions.get(selectedChannel.channelId) : undefined;
  const selectedSessionClips = state.clips.filter((clip) => clip.sessionId === selectedSession?.id);

  const selectSession = (id: string) => setSelectedKey('session:' + id);
  const recordChannel = (id: string) => void perform(async () => {
    const session = await startLiveRecording(id);
    selectSession(session.id);
  }, '라이브 동영상 녹화를 시작했습니다.');

  const updateReservation = async (channel: Channel, enabled: boolean) => {
    if (reservationBusy || !reservationReady) return;
    setReservationBusy(true);
    try {
      const next = await setLiveAutoRecording(channel.channelId, channel.channelName, enabled);
      setState(next);
      message.success(enabled ? '라이브 예약을 설정했습니다.' : '라이브 예약을 해제했습니다.');
    } catch (cause) {
      message.error(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setReservationBusy(false);
    }
  };

  return (
    <div className="live-recording-workspace">
      <Card bordered={false} className="app-panel live-recording-sidebar">
        <div className="live-recording-sidebar-heading">
          <Title level={5} className="!mb-2">채널 목록</Title>
          <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1">
            <Text type="secondary" className="text-xs">방송 확인 간격</Text>
            <Text strong className="text-xs">{state.pollSeconds}초</Text>
          </div>
          <div className="mt-1 flex flex-wrap items-center justify-between gap-x-3 gap-y-1">
            <Text type="secondary" className="text-xs">라이브 동시 녹화</Text>
            <Text strong className="text-xs">{activeSessions.length} / {state.maxConcurrent}개</Text>
          </div>
        </div>
        <div className="live-recording-sidebar-scroll">
          {state.sessions.length > 0 && (
            <div role="tablist" aria-label="라이브 녹화 작업" className="flex flex-col gap-2">
              {state.sessions.slice(0, 100).map((session) => {
                const selected = selectedSession?.id === session.id && selectedKey === 'session:' + session.id;
                const label = session.status === 'recording'
                  ? '녹화 중'
                  : session.status === 'finalizing'
                    ? '저장 중'
                    : '종료';
                return (
                  <button
                    key={session.id}
                    type="button"
                    role="tab"
                    aria-selected={selected}
                    className={'live-recording-list-item live-recording-task-tab' + (selected ? ' is-selected' : '')}
                    onClick={() => selectSession(session.id)}
                  >
                    <span className="min-w-0 flex-1 text-left">
                      <Text strong ellipsis className="block">{session.title || session.channelName || '라이브 방송'}</Text>
                      <Text type="secondary" className="block truncate text-xs">
                        {session.channelName || session.channelId}
                      </Text>
                    </span>
                    <Tag
                      color={session.status === 'recording' ? 'processing' : session.status === 'finalizing' ? 'warning' : undefined}
                      className="!mr-0 shrink-0"
                    >
                      {label}
                    </Tag>
                  </button>
                );
              })}
            </div>
          )}
          <div className="live-recording-list-heading">
            <Text strong>즐겨찾기 목록</Text>
            <Tag>{savedChannels.length}개</Tag>
          </div>
          {savedChannels.length === 0 ? (
            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="저장된 즐겨찾기 채널이 없습니다." />
          ) : (
            <div className="flex flex-col gap-2">
              {savedChannels.map((channel) => {
                const selected = selectedKey === 'channel:' + channel.channelId;
                const activeSession = activeSessions.find((session) => session.channelId === channel.channelId);
                const subscription = subscriptions.get(channel.channelId);
                return (
                  <button
                    key={channel.channelId}
                    type="button"
                    aria-pressed={selected}
                    className={'live-recording-list-item' + (selected ? ' is-selected' : '')}
                    onClick={() => setSelectedKey('channel:' + channel.channelId)}
                  >
                    <Avatar size={36} src={channel.channelImageUrl || undefined}>
                      {channel.channelName.slice(0, 1)}
                    </Avatar>
                    <span className="min-w-0 flex-1 text-left">
                      <Text strong ellipsis className="block">{channel.channelName}</Text>
                      <Text type="secondary" className="block truncate text-xs">
                        {activeSession
                          ? activeSession.status === 'finalizing' ? '파일 생성 중' : '현재 녹화 중'
                          : subscription?.enabled ? '자동 녹화 감시 중' : '라이브 녹화 가능'}
                      </Text>
                    </span>
                    {activeSession ? (
                      <Tag color="processing" className="!mr-0">녹화 중</Tag>
                    ) : subscription?.enabled ? (
                      <Tag className="!mr-0">자동</Tag>
                    ) : null}
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

          {!selectedSession && selectedChannel ? (
            <ChannelRecordingSetup
              key={selectedChannel.channelId}
              channel={selectedChannel}
              reserved={Boolean(selectedSubscription?.enabled)}
              subscriptionError={selectedSubscription?.lastError}
              reservationReady={reservationReady}
              reservationBusy={reservationBusy}
              recordingBusy={busy}
              onReservationChange={(enabled) => void updateReservation(selectedChannel, enabled)}
              onRecord={() => recordChannel(selectedChannel.channelId)}
            />
          ) : !selectedSession ? (
            <Empty description="현재 녹화 중인 작업이 없습니다. 왼쪽 즐겨찾기에서 채널을 선택해 녹화를 시작할 수 있습니다." />
          ) : (
            <div className="flex flex-col gap-4">
              <div className="rounded-xl border border-solid border-slate-500/25 p-5">
                <div className="live-recording-session-header">
                  <RecordingThumbnail url={selectedSession.thumbnailUrl} title={selectedSession.title || '라이브'} />
                  <div className="min-w-0 flex-1">
                    <Space wrap size={8}>
                      <Tag color={selectedSession.status === 'recording' ? 'processing' : undefined}>
                        {statusLabel(selectedSession.status)}
                      </Tag>
                      <Text type="secondary" className="text-xs">{selectedSession.channelName}</Text>
                    </Space>
                    <Title level={5} className="!mb-3 !mt-3">
                      {selectedSession.title || '라이브 방송'}
                    </Title>
                    <ChannelBroadcastStatus channelId={selectedSession.channelId} />
                    {selectedSession.broadcastStartedAt && (
                      <Text className="mb-1 block text-xs">
                        <Text type="secondary">치지직 방송 시작</Text> · {formatChzzkDateTime(selectedSession.broadcastStartedAt)}
                      </Text>
                    )}
                    <Text className="mb-1 block text-xs">
                      <Text type="secondary">녹화 시작</Text> · {formatChzzkDateTime(selectedSession.startedAt)}
                      {formatBroadcastOffset(selectedSession.startedAt, selectedSession.broadcastStartedAt) && (
                        <Text type="secondary"> (방송 +{formatBroadcastOffset(selectedSession.startedAt, selectedSession.broadcastStartedAt)})</Text>
                      )}
                    </Text>
                    <Text className="mb-1 block text-xs">
                      <Text type="secondary">녹화 종료</Text> · {
                        selectedSession.recordingEndedAt || selectedSession.finishedAt
                          ? formatChzzkDateTime(selectedSession.recordingEndedAt || selectedSession.finishedAt)
                          : selectedSession.status === 'recording' ? '녹화 중' : '기록 없음'
                      }
                      {formatBroadcastOffset(selectedSession.recordingEndedAt || selectedSession.finishedAt, selectedSession.broadcastStartedAt) && (
                        <Text type="secondary">
                          {' '}(방송 +{formatBroadcastOffset(selectedSession.recordingEndedAt || selectedSession.finishedAt, selectedSession.broadcastStartedAt)})
                        </Text>
                      )}
                    </Text>
                    <Text type="secondary" className="mt-3 block text-xs">
                      저장된 구간 {formatTime(selectedSession.availableSeconds)}
                    </Text>
                    <Text type="secondary" className="mt-1 block break-all text-xs">
                      {selectedSession.finalPath || selectedSession.directory}
                    </Text>
                    {selectedSession.status === 'recording' && (
                      <Button
                        danger
                        className="!mt-3"
                        loading={busy}
                        onClick={() => void perform(() => stopLiveRecording(selectedSession.id), '녹화 중지를 요청했습니다.')}
                      >
                        녹화 중지
                      </Button>
                    )}
                  </div>
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
