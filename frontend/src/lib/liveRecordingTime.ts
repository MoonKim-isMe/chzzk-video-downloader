function parseChzzkDateTime(value?: string): Date | undefined {
  if (!value?.trim()) return undefined;
  const raw = value.trim();
  const local = /^(\d{4}-\d{2}-\d{2})[ T](\d{2}:\d{2}:\d{2})(\.\d+)?$/.exec(raw);
  const date = new Date(local ? `${local[1]}T${local[2]}${local[3] ?? ''}+09:00` : raw);
  return Number.isNaN(date.getTime()) ? undefined : date;
}

// CHZZK openDate values without an offset are Korean local time.
// Recorder timestamps include their original offset (RFC3339).
export function formatChzzkDateTime(value?: string): string {
  if (!value?.trim()) return '—';

  const date = parseChzzkDateTime(value);
  if (!date) return '시간 정보 없음';

  const parts = new Intl.DateTimeFormat('ko-KR', {
    timeZone: 'Asia/Seoul',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hourCycle: 'h23',
  }).formatToParts(date);
  const part = (name: string) => parts.find((entry) => entry.type === name)?.value ?? '';
  return `${part('year')}.${part('month')}.${part('day')} ${part('hour')}:${part('minute')}:${part('second')} KST`;
}

/** Recording position on the CHZZK broadcast timeline, not on the saved file. */
export function formatBroadcastOffset(recordedAt?: string, broadcastStartedAt?: string): string | undefined {
  const recorded = parseChzzkDateTime(recordedAt);
  const broadcast = parseChzzkDateTime(broadcastStartedAt);
  if (!recorded || !broadcast || recorded.getTime() < broadcast.getTime()) return undefined;
  const seconds = Math.floor((recorded.getTime() - broadcast.getTime()) / 1000);
  return [Math.floor(seconds / 3600), Math.floor(seconds / 60) % 60, seconds % 60]
    .map((part) => String(part).padStart(2, '0')).join(':');
}
