export interface SegmentRange {
  title: string;
  start: string;
  end: string;
}

export function parseSegmentTime(value: string): number | null {
  const text = value.trim();
  if (!/^\d+:\d{1,2}(?::\d{1,2})?$/.test(text)) return null;
  const parts = text.split(':').map(Number);
  if (parts.some((part) => !Number.isSafeInteger(part) || part < 0)) return null;
  const seconds = parts[parts.length - 1];
  if (seconds >= 60) return null;
  if (parts.length === 2) return parts[0] * 60 + seconds;
  if (parts[1] >= 60) return null;
  const total = parts[0] * 3600 + parts[1] * 60 + seconds;
  return Number.isSafeInteger(total) ? total : null;
}

export function formatSegmentTime(totalSeconds: number): string {
  const safe = Math.max(0, Math.floor(totalSeconds));
  const seconds = String(safe % 60).padStart(2, '0');
  const minutes = Math.floor(safe / 60);
  if (minutes < 60) return minutes + ':' + seconds;
  return Math.floor(minutes / 60) + ':' + String(minutes % 60).padStart(2, '0') + ':' + seconds;
}

export function parseChapterList(text: string, duration: number): SegmentRange[] {
  const chapters: { title: string; seconds: number }[] = [];
  for (const [index, rawLine] of text.split(/\r?\n/).entries()) {
    const line = rawLine.trim();
    if (!line) continue;
    const match = line.match(/^(?:[-*•]\s*)?(\d+:\d{1,2}(?::\d{1,2})?)\s*(?:[-–—|]\s*)?(.*)$/);
    if (!match) throw new Error((index + 1) + '번째 줄에 유효한 타임스탬프가 없습니다.');
    const seconds = parseSegmentTime(match[1]);
    if (seconds === null) throw new Error((index + 1) + '번째 줄의 시간이 올바르지 않습니다.');
    if (duration > 0 && seconds >= duration) {
      throw new Error((index + 1) + '번째 줄이 영상 재생시간을 초과했습니다.');
    }
    if (chapters.length > 0 && seconds <= chapters[chapters.length - 1].seconds) {
      throw new Error((index + 1) + '번째 줄의 시작 시간은 이전 줄보다 커야 합니다.');
    }
    chapters.push({ seconds, title: match[2].trim() || '구간 ' + (chapters.length + 1) });
  }
  if (chapters.length === 0) throw new Error('타임스탬프를 한 줄 이상 입력해 주세요.');
  if (!Number.isFinite(duration) || duration <= 0) {
    throw new Error('영상 재생시간을 확인할 수 없어 마지막 종료 시간을 설정하지 못했습니다.');
  }
  return chapters.map((chapter, i) => ({
    title: chapter.title,
    start: formatSegmentTime(chapter.seconds),
    // Display the preceding whole second. A later download stage must
    // interpret the endpoint as inclusive to avoid dropping boundary media.
    end: i + 1 < chapters.length ? formatSegmentTime(chapters[i + 1].seconds - 1) : formatSegmentTime(duration),
  }));
}

export function validateSegmentRange(segment: SegmentRange, duration: number): string | null {
  if (!segment.title.trim()) return '구간 제목을 입력해 주세요.';
  const start = parseSegmentTime(segment.start);
  if (start === null) return '시작 시간을 M:SS 또는 H:MM:SS 형식으로 입력해 주세요.';
  if (duration > 0 && start >= duration) return '시작 시간이 영상 길이를 초과했습니다.';
  const end = parseSegmentTime(segment.end);
  if (end === null) return '종료 시간을 M:SS 또는 H:MM:SS 형식으로 입력해 주세요.';
  if (end < start) return '종료 시간은 시작 시간보다 빨라서는 안 됩니다.';
  if (duration > 0 && end > duration) return '종료 시간이 영상 길이를 초과했습니다.';
  return null;
}
