import assert from 'node:assert/strict';
import test from 'node:test';

import { formatBroadcastOffset, formatLocalDateTime, getSavedBroadcastPositions } from '../src/lib/liveRecordingTime.ts';

test('CHZZK openDate without offset is interpreted as Korean source time', () => {
  assert.equal(formatLocalDateTime('2026-10-23 12:00:00', 'Asia/Seoul'), '2026.10.23 12:00:00');
  assert.equal(formatLocalDateTime('2026-10-23T12:00:00', 'Asia/Seoul'), '2026.10.23 12:00:00');
});

test('browser timezone controls the display without a KST suffix', () => {
  assert.equal(formatLocalDateTime('2026-10-23 12:00:00', 'America/Los_Angeles'), '2026.10.22 20:00:00');
  assert.equal(formatLocalDateTime('2026-10-23T03:00:00Z', 'America/Los_Angeles'), '2026.10.22 20:00:00');
  assert.equal(formatLocalDateTime('2026-10-23T04:00:00+01:00', 'Asia/Seoul'), '2026.10.23 12:00:00');
});

test('missing or invalid times are safe', () => {
  assert.equal(formatLocalDateTime(), '—');
  assert.equal(formatLocalDateTime('not-a-date'), '시간 정보 없음');
});

test('default formatting uses host local timezone', () => {
  const expected = formatLocalDateTime('2026-10-23T03:00:00Z', Intl.DateTimeFormat().resolvedOptions().timeZone);
  assert.equal(formatLocalDateTime('2026-10-23 12:00:00'), expected);
});

test('recording positions use broadcast-relative offsets even in other timezones', () => {
  const opened = '2026-10-23 12:00:00';
  assert.equal(formatBroadcastOffset('2026-10-23T03:15:30Z', opened), '00:15:30');
  assert.equal(formatBroadcastOffset('2026-10-23T04:15:30Z', opened), '01:15:30');
  assert.equal(formatBroadcastOffset('2026-10-23T02:59:59Z', opened), undefined);
  assert.equal(formatBroadcastOffset('not-a-date', opened), undefined);
});

test('saved broadcast positions add completed media duration to the streaming offset', () => {
  assert.deepEqual(
    getSavedBroadcastPositions('2026-10-23T03:15:30Z', '2026-10-23 12:00:00', 90.8),
    { start: '00:15:30', last: '00:17:00' },
  );
  assert.deepEqual(
    getSavedBroadcastPositions('2026-10-23T03:15:30Z', '2026-10-23 12:00:00', 0),
    { start: '00:15:30' },
  );
  assert.equal(getSavedBroadcastPositions('invalid', '2026-10-23 12:00:00', 120), undefined);
  assert.deepEqual(
    getSavedBroadcastPositions('2026-10-23T03:15:30Z', '2026-10-23 12:00:00', Number.NaN),
    { start: '00:15:30' },
  );
});
