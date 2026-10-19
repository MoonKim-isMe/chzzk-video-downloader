import assert from 'node:assert/strict';
import test from 'node:test';

import { formatSegmentTime, parseChapterList, parseSegmentTime, validateSegmentRange } from '../src/features/videos/segmentRanges.ts';

test('parse pasted timestamps into independent chapter segments', () => {
  assert.deepEqual(parseChapterList('2:59 영상 1\n6:54 영상 2\n7:52 영상 3', 3600), [
    { title: '영상 1', start: '2:59', end: '6:53' },
    { title: '영상 2', start: '6:54', end: '7:51' },
    { title: '영상 3', start: '7:52', end: '1:00:00' },
  ]);
});

test('hours, empty titles and introductory chapters', () => {
  assert.deepEqual(parseChapterList('0:00\n1:02:03 본편', 4400), [
    { title: '구간 1', start: '0:00', end: '1:02:02' },
    { title: '본편', start: '1:02:03', end: '1:13:20' },
  ]);
  assert.equal(parseSegmentTime('1:20:35'), 4835);
  assert.equal(formatSegmentTime(4835), '1:20:35');
});

test('reject malformed, unordered, duplicate, and beyond-duration chapter marks', () => {
  for (const text of ['0:77 잘못된 값', '2:00 첫 번째\n1:00 역순', '2:00 하나\n2:00 둘', '0:10 허용\n0:20 길이 초과', 'text only', '']) {
    assert.throws(() => parseChapterList(text, 20));
  }
});

test('manual overlaps allowed while invalid boundaries are rejected', () => {
  assert.equal(validateSegmentRange({ title: 'A', start: '1:00', end: '3:00' }, 600), null);
  assert.equal(validateSegmentRange({ title: 'B', start: '2:00', end: '4:00' }, 600), null);
  assert.equal(validateSegmentRange({ title: 'C', start: '4:00', end: '10:00' }, 600), null);
  assert.match(validateSegmentRange({ title: '', start: '0:00', end: '0:01' }, 600) ?? '', /제목/);
  assert.match(validateSegmentRange({ title: 'X', start: '4:00', end: '2:00' }, 600) ?? '', /종료/);
});

test('last timestamp is a concrete video duration and an unknown duration cannot be assumed', () => {
  assert.deepEqual(parseChapterList('0:00 인트로', 120), [
    { title: '인트로', start: '0:00', end: '2:00' },
  ]);
  assert.throws(() => parseChapterList('0:00 본편', 0), /재생시간/);
  assert.equal(validateSegmentRange({ title: '마지막', start: '1:00', end: '2:00' }, 120), null);
  assert.match(validateSegmentRange({ title: '이전 방식', start: '1:00', end: '끝까지' }, 120) ?? '', /종료/);
  assert.match(validateSegmentRange({ title: '초과', start: '1:00', end: '2:01' }, 120) ?? '', /초과/);
  assert.equal(parseSegmentTime('00:00'), 0);
});
