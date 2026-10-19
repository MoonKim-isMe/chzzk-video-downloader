import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const app = readFileSync(new URL('../src/App.tsx', import.meta.url), 'utf8');
const editor = readFileSync(new URL('../src/features/videos/SegmentDownloadEditor.tsx', import.meta.url), 'utf8');

test('selected segments are registered sequentially, preserving the visible order', () => {
  const start = app.indexOf('const handleQueueSegments = useCallback(');
  const end = app.indexOf('const handleDownloadTaskRemoved = useCallback(', start);
  assert.notEqual(start, -1);
  assert.notEqual(end, -1);
  const handler = app.slice(start, end);
  assert.match(handler, /const requests = segments\.map\(\(segment, index\) =>/);
  assert.match(handler, /position: index \+ 1,/);
  assert.match(handler, /for \(const request of requests\) \{\s*try \{\s*const task = await startDownload\(request\);/);
  assert.doesNotMatch(handler, /Promise\.all(?:Settled)?/);
  assert.match(handler, /setDownloadTasks\(\(current\) => upsertDownloadTask\(current, task\)\)/);
});

test('only selected rows are queued and sequential download ordering is explained in the UI', () => {
  assert.match(editor, /const selectedRows = rows\.filter\(\(row\) => row\.selected\)/);
  assert.match(editor, /selectedRows\.map\(\(\{ title, start, end \}\) => \(\{ title, start, end \}\)\)/);
  assert.match(editor, /목록 순서대로 다운로드합니다/);
  assert.match(editor, /예: 01\. 영상1\.mp4/);
});
