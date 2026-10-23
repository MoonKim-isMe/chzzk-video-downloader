import { Alert, App as AntdApp, Avatar, Button, Card, Checkbox, Empty, Input, Segmented, Space, Switch, Tag, Typography } from 'antd';
import { useCallback, useEffect, useRef, useState } from 'react';

import { clearLiveRecordingHistory, deleteLiveRecordingHistory, extractLiveClip, getChannelLiveStatus, getLiveRecordingState, openDownloadDirectory, setLiveAutoRecording, startLiveRecording, stopLiveRecording } from '../../lib/backend';
import type { LiveSession, LiveState, LiveStatus } from '../../types/live';
import type { Channel } from '../../types/channel';
import { formatBroadcastOffset, formatLocalDateTime, getClipVideoSeconds, getRecordingBroadcastOffsetSeconds, getSavedBroadcastPositions } from '../../lib/liveRecordingTime';
import type { ClipTimeBasis } from '../../lib/liveRecordingTime';

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

  return snapshot?.channelId === channelId ? snapshot : undefined;
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
      <div className="mb-3 flex flex-wrap items-start justify-between gap-4">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <Title level={5} className="!m-0">{channel.channelName || channel.channelId}</Title>
          <BroadcastStatusTag status={status} />
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <Text className="text-sm">라이브 예약</Text>
          <Switch
            aria-label="라이브 예약"
            checked={reserved}
            loading={reservationBusy}
            disabled={!reservationReady || reservationBusy}
            onChange={onReservationChange}
          />
        </div>
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
      <Button type="primary" loading={recordingBusy} disabled={status !== 'live'} onClick={onRecord}>
        지금 동영상 녹화
      </Button>
      {reserved && (
        <Text type="secondary" className="mt-3 block text-xs">
          예약 중입니다. 방송 시작을 감지하면 자동으로 녹화합니다.
        </Text>
      )}
    </div>
  );
}

function LiveBroadcastPreview({
  live,
  showStreamingTime = true,
}: {
  live: LiveStatus;
  showStreamingTime?: boolean;
}) {
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    if (!showStreamingTime) return;
    setNow(Date.now());
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [live.liveId, live.openDate, showStreamingTime]);

  const streamingTime = showStreamingTime
    ? formatBroadcastOffset(new Date(now).toISOString(), live.openDate)
    : undefined;
  const tags = [...new Set(live.tags ?? [])].map((tag) => tag.trim()).filter(Boolean);

  return (
    <div className="live-recording-session-header mb-4">
      <RecordingThumbnail url={live.thumbnailUrl} title={live.liveTitle || '라이브'} />
      <div className="min-w-0">
        <Title level={5} className="!mb-3 !mt-0 break-words">
          {live.liveTitle || '제목 없는 방송'}
        </Title>
        <div className="mb-2">
          <Text type="secondary" className="mr-2 text-xs">방송 시작 시간</Text>
          <Text className="text-sm">{formatLocalDateTime(live.openDate)}</Text>
        </div>
        {showStreamingTime && (
          <div className="mb-3">
            <Text type="secondary" className="mr-2 text-xs">스트리밍 시간</Text>
            <Text className="text-sm tabular-nums" strong>{streamingTime ?? '시간 정보 없음'}</Text>
          </div>
        )}
        {(live.liveCategoryValue || tags.length > 0) && (
          <div className="flex flex-wrap gap-1.5">
            {live.liveCategoryValue && (
              <Tag color="green" className="!m-0">{live.liveCategoryValue}</Tag>
            )}
            {tags.map((tag) => (
              <Tag key={tag} className="!m-0">#{tag.replace(/^#/, '')}</Tag>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

/**
 * Segments expose their total saved media duration, not the wall-clock time
 * of the most recent segment. The final broadcast position is an estimate
 * when the recorder has gaps in reception.
 */
function SavedBroadcastRange({ session }: { session: LiveSession }) {
  const isRecording = session.status === 'recording';
  // FFmpeg progress includes the open segment. Clip extraction deliberately
  // continues to use session.availableSeconds (completed media only).
  const shownSeconds = Math.max(0, Math.floor(isRecording
    ? Math.max(session.availableSeconds, session.recordedSeconds ?? 0)
    : session.availableSeconds));
  const positions = getSavedBroadcastPositions(session.startedAt, session.broadcastStartedAt, shownSeconds);
  const start = positions?.start;
  const lastSaved = positions?.last;

  return (
    <div className="live-recording-saved-range mt-4">
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <Text strong>저장된 방송 구간</Text>
        <Tag color="green" className="!m-0">{isRecording ? '녹화 길이' : '저장 길이'} {formatTime(shownSeconds)}</Tag>
      </div>
      <div className="live-recording-range-points">
        <div className="live-recording-range-point">
          <Text type="secondary" className="block text-xs">저장 시작 · 방송 기준</Text>
          <Text strong className="block tabular-nums text-lg">{start ?? '시간 정보 없음'}</Text>
        </div>
        <div className="live-recording-range-connector" aria-hidden="true" />
        <div className="live-recording-range-point">
          <Text type="secondary" className="block text-xs">마지막 저장 · 방송 기준</Text>
          <Text strong className="block tabular-nums text-lg">
            {lastSaved ?? (shownSeconds <= 0 ? '저장 대기' : '시간 정보 없음')}
          </Text>
        </div>
      </div>
      {start && shownSeconds > 0 && (
        <Text type="secondary" className="mt-3 block text-xs">
          {isRecording
            ? '실제 녹화 진행 기준 추정 위치입니다. 구간 추출은 저장 완료된 영상까지만 가능합니다.'
            : '저장된 영상 길이를 기준으로 계산한 위치입니다. 방송 수신이 끊긴 경우 실제 방송 시각과 차이가 날 수 있습니다.'}
        </Text>
      )}
    </div>
  );
}

function RecordingFolderButton({ directory, compact = false }: { directory: string; compact?: boolean }) {
  const { message } = AntdApp.useApp();
  const [opening, setOpening] = useState(false);

  const openFolder = async () => {
    if (opening) return;
    setOpening(true);
    try {
      await openDownloadDirectory(directory);
    } catch (cause) {
      message.error(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setOpening(false);
    }
  };

  return (
    <Button
      size={compact ? 'small' : undefined}
      loading={opening}
      disabled={!directory}
      onClick={() => void openFolder()}
    >
      폴더 열기
    </Button>
  );
}

function RecordingSessionOverview({ session, busy, hasActiveRecording, onStop, onRecord }: {
  session: LiveSession;
  busy: boolean;
  hasActiveRecording: boolean;
  onStop: () => void;
  onRecord: () => void;
}) {
  const snapshot = useChannelBroadcastStatus(session.channelId);
  const currentLive = snapshot?.live?.liveId === session.liveId ? snapshot.live : undefined;
  const canRecordAgain = Boolean(currentLive) && (
    session.status === 'completed' || session.status === 'failed' || session.status === 'interrupted'
  );

  const broadcast: LiveStatus = {
    channelId: session.channelId,
    channelName: session.channelName,
    liveId: session.liveId,
    isLive: Boolean(currentLive),
    liveTitle: session.title || currentLive?.liveTitle || '',
    openDate: session.broadcastStartedAt || currentLive?.openDate || '',
    thumbnailUrl: session.thumbnailUrl || currentLive?.thumbnailUrl,
    tags: session.tags?.length ? session.tags : (currentLive?.tags ?? []),
    liveCategoryValue: session.liveCategoryValue || currentLive?.liveCategoryValue,
  };

  return (
    <div className="rounded-xl border border-solid border-slate-500/25 p-5">
      <div className="mb-4 flex flex-wrap items-center gap-2">
        <Title level={5} className="!m-0">{session.channelName || session.channelId}</Title>
        <Tag
          color={session.status === 'recording' ? 'processing' : undefined}
          className={session.status === 'recording' ? '!m-0 live-recording-active-tag' : '!m-0'}
        >
          {statusLabel(session.status)}
        </Tag>
        <BroadcastStatusTag status={snapshot?.status} />
      </div>
      <LiveBroadcastPreview live={broadcast} showStreamingTime={Boolean(currentLive)} />
      <SavedBroadcastRange session={session} />
      {session.status !== 'recording' && (
        <div className="mt-3 flex flex-wrap items-center gap-2">
          {canRecordAgain && (
            <Button
              type="primary"
              loading={busy}
              disabled={busy || hasActiveRecording}
              onClick={onRecord}
            >
              추가 녹화
            </Button>
          )}
          <RecordingFolderButton directory={session.directory} />
        </div>
      )}
      {(session.recordingEndedAt || session.finishedAt) && (
        <Text type="secondary" className="mt-3 block text-xs">
          녹화 종료 · {formatLocalDateTime(session.recordingEndedAt || session.finishedAt)}
        </Text>
      )}
      {session.status === 'recording' && (
        <Button
          danger
          className="!mt-4"
          loading={busy}
          onClick={onStop}
        >
          녹화 중지
        </Button>
      )}
      {session.error && (
        <Alert type="warning" showIcon message={session.error} className="!mt-4" />
      )}
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
  const [timeBasis, setTimeBasis] = useState<ClipTimeBasis>('video');
  const [exact, setExact] = useState(false);
  const [working, setWorking] = useState(false);
  const broadcastOffset = getRecordingBroadcastOffsetSeconds(session.startedAt, session.broadcastStartedAt);
  const rangeStart = timeBasis === 'broadcast' ? (broadcastOffset ?? 0) : 0;
  const rangeEnd = rangeStart + session.availableSeconds;

  const changeTimeBasis = (next: ClipTimeBasis) => {
    if (next === timeBasis || (next === 'broadcast' && broadcastOffset === undefined)) return;
    // Preserve the selected media segment while changing its clock. Invalid
    // draft input stays unchanged for correction by the user.
    const shift = (next === 'broadcast' ? 1 : -1) * (broadcastOffset ?? 0);
    const convertInput = (value: string) => {
      const seconds = toSeconds(value);
      return seconds === undefined ? value : formatTime(Math.max(0, seconds + shift));
    };
    setStart((value) => convertInput(value));
    setEnd((value) => convertInput(value));
    setTimeBasis(next);
  };

  const extract = async () => {
    const inputStart = toSeconds(start);
    const inputEnd = toSeconds(end);
    const a = inputStart === undefined ? undefined : getClipVideoSeconds(inputStart, timeBasis, broadcastOffset);
    const b = inputEnd === undefined ? undefined : getClipVideoSeconds(inputEnd, timeBasis, broadcastOffset);
    if (a === undefined || b === undefined || b <= a || b > session.availableSeconds) {
      message.warning('선택한 시간 기준의 추출 가능 구간 안에서 시작·종료 시간을 입력해 주세요.');
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
    <div>
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <Title level={5} className="!m-0">녹화 구간 추출</Title>
        <Segmented
          size="small"
          aria-label="추출 시간 기준"
          value={timeBasis}
          onChange={(value) => changeTimeBasis(value === 'broadcast' ? 'broadcast' : 'video')}
          options={[
            { label: '방송 시간 기준', value: 'broadcast', disabled: broadcastOffset === undefined },
            { label: '영상 시간 기준', value: 'video' },
          ]}
        />
      </div>
      <Text type="secondary" className="mb-4 block text-xs">
        {timeBasis === 'broadcast'
          ? '방송 시작 후 경과 시간으로 입력합니다. 수신이 끊긴 구간이 있으면 실제 방송 시각과 차이가 날 수 있습니다.'
          : '녹화된 영상의 시작을 00:00:00으로 계산합니다.'}
        {' '}저장 완료된 구간만 추출할 수 있으며 녹화는 중단되지 않습니다.
      </Text>
      {broadcastOffset === undefined && (
        <Text type="secondary" className="mb-3 block text-xs">
          방송 시작 시각을 확인할 수 없어 방송 시간 기준을 사용할 수 없습니다.
        </Text>
      )}
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
        {timeBasis === 'broadcast' ? '방송 시간 기준' : '영상 시간 기준'} 추출 가능: {formatTime(rangeStart)} ~ {formatTime(rangeEnd)}
      </Text>
    </div>
  );
}

export default function LiveRecordingPanel({ savedChannels }: { savedChannels: Channel[] }) {
  const { message } = AntdApp.useApp();
  const [state, setState] = useState<LiveState>(initialState);
  const [error, setError] = useState('');
  const [selectedKey, setSelectedKey] = useState<string>();
  const [sidebarTab, setSidebarTab] = useState<'channels' | 'history'>('channels');
  const sidebarTabRef = useRef<'channels' | 'history'>('channels');
  const [deletingId, setDeletingId] = useState<string>();
  const [clearingHistory, setClearingHistory] = useState(false);
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
    let timer: number | undefined;
    const read = async () => {
      let interval = 3000;
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
          if (newRecording && sidebarTabRef.current === 'channels') {
            setSelectedKey((current) => current ?? 'session:' + newRecording.id);
          }
          setState(next);
          setReservationReady(true);
          setError('');
          if (next.sessions.some((session) => session.status === 'recording')) interval = 1000;
        }
      } catch (cause) {
        if (active) setError(cause instanceof Error ? cause.message : String(cause));
      } finally {
        if (active) timer = window.setTimeout(() => { void read(); }, interval);
      }
    };
    void read();
    return () => {
      active = false;
      if (timer !== undefined) window.clearTimeout(timer);
    };
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
  const historySessions = state.sessions.filter((session) =>
    session.status !== 'recording' && session.status !== 'finalizing',
  );
  const historyBusy = Boolean(deletingId) || clearingHistory;
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
  const changeSidebarTab = (next: 'channels' | 'history') => {
    sidebarTabRef.current = next;
    setSidebarTab(next);
    const first = next === 'history'
      ? historySessions[0] ? 'session:' + historySessions[0].id : undefined
      : activeSessions[0] ? 'session:' + activeSessions[0].id
        : savedChannels[0] ? 'channel:' + savedChannels[0].channelId : undefined;
    setSelectedKey(first);
  };
  const recordChannel = (id: string) => void perform(async () => {
    const session = await startLiveRecording(id);
    sidebarTabRef.current = 'channels';
    setSidebarTab('channels');
    selectSession(session.id);
  }, '라이브 동영상 녹화를 시작했습니다.');

  const removeHistory = async (id: string) => {
    if (historyBusy) return;
    setDeletingId(id);
    try {
      const next = await deleteLiveRecordingHistory(id);
      setState(next);
      const remaining = next.sessions.find((session) =>
        session.status !== 'recording' && session.status !== 'finalizing',
      );
      setSelectedKey((current) => current === 'session:' + id
        ? remaining ? 'session:' + remaining.id : undefined
        : current);
      message.success('녹화 기록을 목록에서 삭제했습니다. 저장된 파일은 유지됩니다.');
    } catch (cause) {
      message.error(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setDeletingId(undefined);
    }
  };
  const clearHistory = async () => {
    if (historyBusy || historySessions.length === 0) return;
    setClearingHistory(true);
    try {
      const next = await clearLiveRecordingHistory();
      setState(next);
      setSelectedKey((current) => {
        const id = current?.startsWith('session:') ? current.slice('session:'.length) : undefined;
        return id && !next.sessions.some((session) => session.id === id) ? undefined : current;
      });
      message.success('녹화 기록을 일괄 삭제했습니다. 저장된 파일은 유지됩니다.');
    } catch (cause) {
      message.error(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setClearingHistory(false);
    }
  };

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
          <Segmented
            block
            value={sidebarTab}
            onChange={(value) => changeSidebarTab(value === 'history' ? 'history' : 'channels')}
            options={[
              { label: '채널 목록', value: 'channels' },
              { label: `녹화 기록 (${historySessions.length})`, value: 'history' },
            ]}
          />
          {sidebarTab === 'channels' && (
            <div className="mt-3">
              <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1">
                <Text type="secondary" className="text-xs">방송 확인 간격</Text>
                <Text strong className="text-xs">{state.pollSeconds}초</Text>
              </div>
              <div className="mt-1 flex flex-wrap items-center justify-between gap-x-3 gap-y-1">
                <Text type="secondary" className="text-xs">라이브 동시 녹화</Text>
                <Text strong className="text-xs">{activeSessions.length} / {state.maxConcurrent}개</Text>
              </div>
            </div>
          )}
        </div>
        <div className="live-recording-sidebar-scroll">
          {sidebarTab === 'channels' ? (
            <>
              <div className="live-recording-list-heading live-recording-list-heading-first">
                <Text strong>진행 중인 녹화</Text>
                <Tag className="!m-0">{activeSessions.length}개</Tag>
              </div>
              {activeSessions.length === 0 ? (
                <Text type="secondary" className="block text-xs">현재 진행 중인 녹화가 없습니다.</Text>
              ) : (
                <div role="tablist" aria-label="진행 중인 라이브 녹화" className="flex flex-col gap-2">
                  {activeSessions.map((session) => {
                    const selected = selectedKey === 'session:' + session.id;
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
                          color={session.status === 'recording' ? 'processing' : 'warning'}
                          className={'!mr-0 shrink-0' + (session.status === 'recording' ? ' live-recording-active-tag' : '')}
                        >
                          {session.status === 'recording' ? '녹화 중' : '저장 중'}
                        </Tag>
                      </button>
                    );
                  })}
                </div>
              )}

              <div className="live-recording-list-heading">
                <Text strong>즐겨찾기 목록</Text>
                <Tag className="!m-0">{savedChannels.length}개</Tag>
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
                              : subscription?.enabled ? '라이브 예약 중' : '라이브 녹화 가능'}
                          </Text>
                        </span>
                        {activeSession ? (
                          <Tag color="processing" className="!mr-0 live-recording-active-tag">녹화 중</Tag>
                        ) : subscription?.enabled ? (
                          <Tag className="!mr-0">예약</Tag>
                        ) : null}
                      </button>
                    );
                  })}
                </div>
              )}
            </>
          ) : (
            <>
              <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
                <Text strong>녹화 기록</Text>
                <Button
                  size="small"
                  disabled={historySessions.length === 0 || historyBusy}
                  loading={clearingHistory}
                  onClick={() => void clearHistory()}
                >
                  일괄 삭제
                </Button>
              </div>
              <Text type="secondary" className="mb-4 block text-xs">
                목록에서만 삭제됩니다. 저장된 영상 파일은 유지됩니다.
              </Text>
              {historySessions.length === 0 ? (
                <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="녹화 기록이 없습니다." />
              ) : (
                <div className="flex flex-col gap-2">
                  {historySessions.map((session) => {
                    const selected = selectedKey === 'session:' + session.id;
                    const historyLabel = session.status === 'completed'
                      ? '완료'
                      : session.status === 'interrupted' ? '중단' : statusLabel(session.status);
                    return (
                      <div key={session.id} className={'live-recording-list-item live-recording-history-row' + (selected ? ' is-selected' : '')}>
                        <button
                          type="button"
                          className="live-recording-history-select"
                          aria-pressed={selected}
                          onClick={() => selectSession(session.id)}
                        >
                          <Text strong ellipsis className="block w-full text-left">
                            {session.title || session.channelName || '라이브 방송'}
                          </Text>
                          <span className="flex min-w-0 items-center gap-2 pr-[94px]">
                            <Text type="secondary" className="min-w-0 truncate text-xs">
                              {session.channelName || session.channelId}
                            </Text>
                            <Tag className="!m-0 shrink-0">{historyLabel}</Tag>
                          </span>
                        </button>
                        <Button
                          size="small"
                          className="live-recording-history-delete"
                          loading={deletingId === session.id}
                          disabled={historyBusy}
                          onClick={() => void removeHistory(session.id)}
                        >
                          목록 삭제
                        </Button>
                      </div>
                    );
                  })}
                </div>
              )}
            </>
          )}
        </div>
      </Card>

      <Card bordered={false} className="app-panel live-recording-detail">
        <div className="live-recording-detail-heading">
          <div className="min-w-0">
            <Title level={4} className="!mb-1">라이브 녹화</Title>
            <Text type="secondary" className="text-xs">방송 녹화와 저장된 작업 관리</Text>
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
            <Empty description={sidebarTab === 'history'
              ? '선택된 녹화 기록이 없습니다. 왼쪽 목록에서 녹화 기록을 선택해 주세요.'
              : '현재 녹화 중인 작업이 없습니다. 왼쪽 즐겨찾기에서 채널을 선택해 녹화를 시작할 수 있습니다.'} />
          ) : (
            <div className="flex flex-col gap-4">
              <RecordingSessionOverview
                key={selectedSession.id}
                session={selectedSession}
                busy={busy}
                hasActiveRecording={activeSessions.some((session) => session.channelId === selectedSession.channelId)}
                onStop={() => void perform(
                  () => stopLiveRecording(selectedSession.id),
                  '녹화 중지를 요청했습니다.',
                )}
                onRecord={() => recordChannel(selectedSession.channelId)}
              />
              <section className="rounded-xl border border-solid border-slate-500/25 p-5">
                <ClipEditor key={selectedSession.id} session={selectedSession} onUpdated={() => void refresh()} />
                <div className="mt-5 border-t border-solid border-slate-500/20 pt-4">
                  <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
                    <Text strong>추출 내역</Text>
                    <RecordingFolderButton
                      key={selectedSession.id}
                      directory={selectedSession.directory}
                      compact
                    />
                  </div>
                {selectedSessionClips.length === 0 ? (
                  <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="추출한 구간이 없습니다." />
                ) : (
                  <div className="flex flex-col gap-2">
                    {selectedSessionClips.map((clip) => (
                      <div key={clip.id} className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-solid border-slate-500/25 p-3">
                        <div className="min-w-0">
                          <Text strong className="block">{clip.title}</Text>
                          <Text type="secondary" className="block text-xs">
                            영상 기준 {formatTime(clip.startSeconds)} ~ {formatTime(clip.endSeconds)}
                          </Text>
                          {clip.error && (
                            <Text type="danger" className="block text-xs">{clip.error}</Text>
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
              </section>
            </div>
          )}
        </div>
      </Card>
    </div>
  );
}
