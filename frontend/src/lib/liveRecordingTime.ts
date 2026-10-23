function parseChzzkDateTime(value?: string): Date | undefined {
  if (!value?.trim()) return undefined;
  const raw = value.trim();
  const local = /^(\d{4}-\d{2}-\d{2})[ T](\d{2}:\d{2}:\d{2})(\.\d+)?$/.exec(raw);
  const date = new Date(local ? `${local[1]}T${local[2]}${local[3] ?? ''}+09:00` : raw);
  return Number.isNaN(date.getTime()) ? undefined : date;
}

// CHZZK openDate values without an offset are Korean local time.
// Recorder timestamps include their original offset (RFC3339).
export function formatLocalDateTime(value?: string, timeZone?: string): string {
  if (!value?.trim()) return '—';

  const date = parseChzzkDateTime(value);
  if (!date) return '시간 정보 없음';

  const parts = new Intl.DateTimeFormat('ko-KR', {
    ...(timeZone ? { timeZone } : {}),
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hourCycle: 'h23',
  }).formatToParts(date);
  const part = (name: string) => parts.find((entry) => entry.type === name)?.value ?? '';
  return `${part('year')}.${part('month')}.${part('day')} ${part('hour')}:${part('minute')}:${part('second')}`;
}

/** Seconds already elapsed on the broadcast timeline when recording began.
 * CHZZK openDate without timezone is KST, but recording timestamps are RFC3339.
 */
export function getRecordingBroadcastOffsetSeconds(
  recordingStartedAt?: string,
  broadcastStartedAt?: string,
): number | undefined {
  const recording = parseChzzkDateTime(recordingStartedAt);
  const broadcast = parseChzzkDateTime(broadcastStartedAt);
  if (!recording || !broadcast || recording.getTime() < broadcast.getTime()) return undefined;
  return Math.floor((recording.getTime() - broadcast.getTime()) / 1000);
}

export type ClipTimeBasis = 'video' | 'broadcast';

/** Convert a user-entered clock on either timeline to seconds in the saved file. */
export function getClipVideoSeconds(
  seconds: number,
  basis: ClipTimeBasis,
  broadcastOffset?: number,
): number | undefined {
  if (!Number.isFinite(seconds) || seconds < 0) return undefined;
  if (basis === 'video') return seconds;
  if (broadcastOffset === undefined || !Number.isFinite(broadcastOffset)) return undefined;
  const videoSeconds = seconds - broadcastOffset;
  return videoSeconds >= 0 ? videoSeconds : undefined;
}

/** Recording position on the CHZZK broadcast timeline, not on the saved file. */
export function formatBroadcastOffset(recordedAt?: string, broadcastStartedAt?: string): string | undefined {
  const seconds = getRecordingBroadcastOffsetSeconds(recordedAt, broadcastStartedAt);
  if (seconds === undefined) return undefined;
  return [Math.floor(seconds / 3600), Math.floor(seconds / 60) % 60, seconds % 60]
    .map((part) => String(part).padStart(2, '0')).join(':');
}

/** Estimated positions on the original broadcast timeline, using saved media duration. */
export function getSavedBroadcastPositions(
  recordingStartedAt?: string,
  broadcastStartedAt?: string,
  availableSeconds = 0,
): { start: string; last?: string } | undefined {
  const start = formatBroadcastOffset(recordingStartedAt, broadcastStartedAt);
  if (!start) return undefined;

  const saved = Math.max(0, Math.floor(Number.isFinite(availableSeconds) ? availableSeconds : 0));
  if (saved === 0) return { start };

  const [hours, minutes, seconds] = start.split(':').map(Number);
  const position = hours * 3600 + minutes * 60 + seconds + saved;
  return {
    start,
    last: [Math.floor(position / 3600), Math.floor(position / 60) % 60, position % 60]
      .map((part) => String(part).padStart(2, '0')).join(':'),
  };
}
