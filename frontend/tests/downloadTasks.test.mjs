import assert from 'node:assert/strict';
import test from 'node:test';

import {
  mergeDownloadTaskSnapshot,
  pickFresherDownloadTask,
  upsertDownloadTask,
} from '../src/lib/downloadTasks.ts';

const progress = { status: '', percent: 0, downloadedBytes: 0, totalBytes: 0, totalBytesEstimated: false, speedBytesPerSecond: 0, etaSeconds: 0 };

function task(status, overrides = {}) {
  return { taskId: 'segment-1', status, progress: { ...progress }, ...overrides };
}

test('a prepared segment advances through queued and running without regressing', () => {
  const prepared = task('prepared');
  const queued = task('queued');
  const running = task('running', { progress: { ...progress, percent: 28, downloadedBytes: 1024 } });

  assert.equal(pickFresherDownloadTask(prepared, queued).status, 'queued');
  assert.equal(pickFresherDownloadTask(queued, prepared).status, 'queued');
  assert.equal(pickFresherDownloadTask(running, prepared).status, 'running');

  const tasks = upsertDownloadTask([prepared], queued);
  assert.equal(tasks[0].status, 'queued');
  assert.equal(upsertDownloadTask(tasks, running)[0].status, 'running');
});

test('stale prepared snapshots cannot overwrite active or finished segment tasks', () => {
  const current = task('completed', { finishedAt: '2026-10-19T10:00:00Z' });
  const stale = task('prepared');
  assert.equal(mergeDownloadTaskSnapshot([current], [stale])[0].status, 'completed');
  assert.equal(mergeDownloadTaskSnapshot([task('running')], [stale])[0].status, 'running');
});

test('terminal task precedence and newer running progress remain unchanged', () => {
  const earlier = task('completed', { finishedAt: '2026-10-19T09:00:00Z' });
  const later = task('completed', { finishedAt: '2026-10-19T10:00:00Z' });
  assert.equal(pickFresherDownloadTask(earlier, later), later);
  assert.equal(pickFresherDownloadTask(later, earlier), later);

  const running = task('running', { progress: { ...progress, percent: 60, downloadedBytes: 2048 } });
  const staleRunning = task('running', { progress: { ...progress, percent: 40, downloadedBytes: 1024 } });
  assert.equal(pickFresherDownloadTask(running, staleRunning), running);
});
