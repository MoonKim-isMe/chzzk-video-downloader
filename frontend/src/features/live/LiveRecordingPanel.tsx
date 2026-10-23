import { Alert, App as AntdApp, Button, Card, Checkbox, Empty, Input, InputNumber, Select, Space, Switch, Tag, Typography } from 'antd';
import { useCallback, useEffect, useState } from 'react';
import {
 configureLiveRecording,
 extractLiveClip,
 getLiveRecordingState,
 removeLiveAutoRecording,
 setLiveAutoRecording,
 startLiveRecording,
 stopLiveRecording,
} from '../../lib/backend';
import type { Channel } from '../../types/channel';
import type { LiveSession, LiveState } from '../../types/live';

const { Text, Title } = Typography;
const empty: LiveState = { pollSeconds: 30, maxConcurrent: 2, subscriptions: [], sessions: [], clips: [] };
const channelIdPattern = /^[a-f0-9]{32}$/i;

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
 return ({ recording: '녹화 중', finalizing: '파일 생성 중', completed: '녹화 완료', failed: '실패', interrupted: '녹화 중단' })[status];
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
     message.success('구간 추출 작업을 시작했습니다. 원본 라이브 녹화는 계속됩니다.');
     onUpdated();
   } catch (cause) {
     message.error(cause instanceof Error ? cause.message : String(cause));
   } finally { setWorking(false); }
 };
 return (
   <div className="mt-3 rounded-lg border border-solid border-slate-500/25 p-3">
     <Text strong className="mb-2 block">녹화된 구간 추출</Text>
     <Space size={8} wrap>
       <Input aria-label="추출 파일 제목" value={title} onChange={(event) => setTitle(event.target.value)} placeholder="구간 제목" style={{ width: 160 }} />
       <Input aria-label="시작 시간" value={start} onChange={(event) => setStart(event.target.value)} placeholder="00:00:00" style={{ width: 106 }} />
       <Text type="secondary">~</Text>
       <Input aria-label="종료 시간" value={end} onChange={(event) => setEnd(event.target.value)} placeholder="00:01:00" style={{ width: 106 }} />
       <Button type="primary" onClick={() => void extract()} loading={working} disabled={session.availableSeconds === 0}>MP4 추출</Button>
     </Space>
     <div className="mt-2">
       <Checkbox checked={exact} onChange={(event) => setExact(event.target.checked)}>정확한 시간 경계 (재인코딩)</Checkbox>
       <Text type="secondary" className="ml-3 text-xs">추출 가능: 00:00:00 ~ {formatTime(session.availableSeconds)}</Text>
     </div>
   </div>
 );
}

export default function LiveRecordingPanel({ savedChannels }: { savedChannels: Channel[] }) {
 const { message } = AntdApp.useApp();
 const [state, setState] = useState<LiveState>(empty);
 const [error, setError] = useState('');
 const [busy, setBusy] = useState('');
 const [channelInput, setChannelInput] = useState('');
 const refresh = useCallback(async () => { setState(await getLiveRecordingState()); }, []);
 useEffect(() => {
   let active = true;
   const read = async () => {
     try { const next = await getLiveRecordingState(); if (active) { setState(next); setError(''); } }
     catch (cause) { if (active) setError(cause instanceof Error ? cause.message : String(cause)); }
   };
   void read();
   const timer = window.setInterval(() => { void read(); }, 3000);
   return () => { active = false; window.clearInterval(timer); };
 }, []);
 const run = async (key: string, action: () => Promise<unknown>) => {
   setBusy(key);
   try { await action(); await refresh(); }
   catch (cause) { message.error(cause instanceof Error ? cause.message : String(cause)); }
   finally { setBusy(''); }
 };
 const subscribed = new Map(state.subscriptions.map((entry) => [entry.channelId, entry]));
 const channels = [
   ...savedChannels.map((entry) => ({ channelId: entry.channelId, channelName: entry.channelName })),
   ...state.subscriptions.filter((entry) => !savedChannels.some((channel) => channel.channelId === entry.channelId)),
 ];
 const liveCount = state.sessions.filter((session) => session.status === 'recording' || session.status === 'finalizing').length;
 return (
   <div className="mx-auto flex max-w-[1150px] flex-col gap-4 pb-8">
     <Card bordered={false} className="app-panel">
       <div className="flex flex-wrap items-center justify-between gap-3">
         <div>
           <Title level={4} className="!mb-1">라이브 자동 녹화</Title>
           <Text type="secondary">앱이 실행 중일 때 등록된 채널의 방송을 감지하여 자동으로 녹화합니다.</Text>
         </div>
         <Tag color="green">라이브 {liveCount} / {state.maxConcurrent} · VOD 별도</Tag>
       </div>
       {error && <Alert className="!mt-3" type="error" showIcon message={error} />}
       <Space wrap size={12} className="!mt-4">
         <Text>방송 확인 간격</Text>
         <Select
           aria-label="방송 확인 간격"
           value={state.pollSeconds}
           style={{ width: 120 }}
           options={[15,30,60].map((n) => ({ value: n, label: n + '초' }))}
           onChange={(value) => void run('config', () => configureLiveRecording(value, state.maxConcurrent))}
         />
         <Text>최대 동시 라이브 녹화</Text>
         <InputNumber
           aria-label="동시 라이브 녹화 수"
           min={1} max={4} value={state.maxConcurrent}
           onChange={(value) => { if (value !== null) void run('config', () => configureLiveRecording(state.pollSeconds, value)); }}
         />
       </Space>
       <Text type="secondary" className="!mt-3 block text-xs">VOD 다운로드 제한과 독립적으로 동작합니다. 앱을 완전히 종료하면 자동 감시도 종료됩니다.</Text>
     </Card>
     <Card bordered={false} className="app-panel" title="자동 녹화 채널">
       <div className="mb-4 flex flex-wrap gap-2">
         <Input
           value={channelInput}
           onChange={(event) => setChannelInput(event.target.value)}
           placeholder="채널 ID 또는 https://chzzk.naver.com/live/채널ID"
           style={{ maxWidth: 490 }}
           onPressEnter={() => {
             const id = channelInput.trim().split('/').filter(Boolean).pop()?.toLowerCase() ?? '';
             if (!channelIdPattern.test(id)) return message.warning('올바른 채널 주소를 입력해 주세요.');
             void run('add', () => setLiveAutoRecording(id, '', true));
             setChannelInput('');
           }}
         />
         <Button
           type="primary"
           loading={busy === 'add'}
           onClick={() => {
             const id = channelInput.trim().split('/').filter(Boolean).pop()?.toLowerCase() ?? '';
             if (!channelIdPattern.test(id)) { message.warning('올바른 채널 주소를 입력해 주세요.'); return; }
             void run('add', () => setLiveAutoRecording(id, '', true));
             setChannelInput('');
           }}
         >채널 추가</Button>
       </div>
       {channels.length === 0 && <Empty description="북마크한 채널을 자동 녹화 대상으로 등록하거나 채널 URL을 입력해 주세요." />}
       <div className="flex flex-col gap-2">
         {channels.map((channel) => {
           const subscription = subscribed.get(channel.channelId);
           const active = state.sessions.find((session) => session.channelId === channel.channelId && session.status === 'recording');
           return (
             <div key={channel.channelId} className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-solid border-slate-500/20 p-3">
               <div className="min-w-0">
                 <Text strong className="block">{subscription?.channelName || channel.channelName || channel.channelId}</Text>
                 <Text type="secondary" className="block text-xs">{channel.channelId}</Text>
                 {subscription?.lastError && <Text type="danger" className="block text-xs">{subscription.lastError}</Text>}
               </div>
               <Space wrap>
                 <Text type="secondary">자동 녹화</Text>
                 <Switch checked={subscription?.enabled ?? false} loading={busy === channel.channelId} onChange={(enabled) => void run(channel.channelId, () => setLiveAutoRecording(channel.channelId, channel.channelName, enabled))} />
                 <Button size="small" disabled={!!active} onClick={() => void run('manual-' + channel.channelId, () => startLiveRecording(channel.channelId))}>지금 녹화</Button>
                 {subscription && <Button size="small" type="text" danger onClick={() => void run('remove-' + channel.channelId, () => removeLiveAutoRecording(channel.channelId))}>감시 제거</Button>}
               </Space>
             </div>
           );
         })}
       </div>
     </Card>
     <Card bordered={false} className="app-panel" title="라이브 녹화 내역">
       {state.sessions.length === 0 && <Empty description="녹화 내역이 없습니다." />}
       <div className="flex flex-col gap-3">
         {state.sessions.slice(0, 50).map((session) => (
           <div key={session.id} className="rounded-xl border border-solid border-slate-500/25 p-4">
             <div className="flex flex-wrap items-start justify-between gap-2">
               <div className="min-w-0">
                 <Space wrap><Text strong>{session.title || session.channelName}</Text><Tag color={session.status === 'recording' ? 'green' : 'default'}>{statusLabel(session.status)}</Tag></Space>
                 <Text type="secondary" className="mt-1 block text-xs">{session.channelName} · 저장된 구간 {formatTime(session.availableSeconds)}</Text>
                 <Text type="secondary" className="mt-1 block break-all text-xs">{session.finalPath || session.directory}</Text>
               </div>
               {session.status === 'recording' && <Button danger onClick={() => void run('stop-' + session.id, () => stopLiveRecording(session.id))}>녹화 중지</Button>}
             </div>
             {session.error && <Alert className="!mt-3" message={session.error} type="warning" showIcon />}
             <ClipEditor session={session} onUpdated={() => void refresh()} />
             {state.clips.filter((clip) => clip.sessionId === session.id).map((clip) => (
               <div key={clip.id} className="mt-2 flex flex-wrap items-center justify-between gap-2">
                 <Text>{clip.title} · {formatTime(clip.startSeconds)} ~ {formatTime(clip.endSeconds)}</Text>
                 <Tag color={clip.status === 'completed' ? 'success' : clip.status === 'failed' ? 'error' : 'processing'}>{({ queued: '대기 중', running: '추출 중', completed: '완료', failed: '실패' })[clip.status]}</Tag>
                 {(clip.filePath || clip.error) && <Text type={clip.error ? 'danger' : 'secondary'} className="break-all text-xs">{clip.filePath || clip.error}</Text>}
               </div>
             ))}
           </div>
         ))}
       </div>
     </Card>
   </div>
 );
}
