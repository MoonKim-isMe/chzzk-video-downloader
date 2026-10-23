import assert from 'node:assert/strict';
import test from 'node:test';

import { formatBroadcastOffset, formatChzzkDateTime } from '../src/lib/liveRecordingTime.ts';

test('CHZZK broadcast openDate is interpreted in Korean local time', () => {
  assert.equal(formatChzzkDateTime('2026-10-23 12:00:00'), '2026.10.23 12:00:00 KST');
  assert.equal(formatChzzkDateTime('2026-10-23T12:00:00'), '2026.10.23 12:00:00 KST');
});

test('recorder UTC and offset times are converted to CHZZK KST', () => {
  assert.equal(formatChzzkDateTime('2026-10-23T03:00:00Z'), '2026.10.23 12:00:00 KST');
  assert.equal(formatChzzkDateTime('2026-10-23T04:00:00+01:00'), '2026.10.23 12:00:00 KST');
});

test('missing or invalid historical time metadata is safe', () => {
  assert.equal(formatChzzkDateTime(), '—');
  assert.equal(formatChzzkDateTime('not-a-date'), '시간 정보 없음');
});

test('recording start and stop positions follow CHZZK broadcast start', () => {
  const opened = '2026-10-23 12:00:00';
  assert.equal(formatBroadcastOffset('2026-10-23T03:15:30Z', opened), '00:15:30');
  assert.equal(formatBroadcastOffset('2026-10-23T04:15:30Z', opened), '01:15:30');
  assert.equal(formatBroadcastOffset('2026-10-23T02:59:59Z', opened), undefined);
  assert.equal(formatBroadcastOffset('not-a-date', opened), undefined);
});
