import assert from 'node:assert/strict';
import test from 'node:test';

import {
  getDownloadRequestCounts,
  getRemainingDownloadCount,
  isResumableDownloadTask,
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


test('download tab counts only actionable remaining jobs', () => {
  const tasks = [
    task('prepared', { taskId: 'prepared' }),
    task('queued', { taskId: 'queued' }),
    task('running', { taskId: 'running' }),
    task('failed', { taskId: 'retry-1', errorCode: 'source_url_missing' }),
    task('failed', { taskId: 'retry-2', errorCode: 'hls_download_stalled' }),
    task('failed', { taskId: 'retry-3', errorCode: 'interrupted' }),
    task('failed', { taskId: 'not-retryable', errorCode: 'authentication_required' }),
    task('completed', { taskId: 'completed' }),
    task('cancelled', { taskId: 'cancelled' }),
  ];
  assert.equal(getRemainingDownloadCount(tasks), 6);
  assert.equal(isResumableDownloadTask(tasks[3]), true);
  assert.equal(isResumableDownloadTask(tasks[6]), false);
  assert.equal(getRemainingDownloadCount(tasks.filter((item) => item.taskId !== 'retry-2')), 5);
});

test('request completion and total count the four visible status groups', () => {
  const tasks = [
    task('prepared', { taskId: 'prepared' }),
    task('queued', { taskId: 'queued' }),
    task('running', { taskId: 'running' }),
    task('completed', { taskId: 'completed-1' }),
    task('completed', { taskId: 'completed-2' }),
    task('failed', { taskId: 'resumable', errorCode: 'source_url_missing' }),
    task('failed', { taskId: 'not-resumable', errorCode: 'authentication_required' }),
    task('cancelled', { taskId: 'cancelled' }),
  ];

  assert.deepEqual(getDownloadRequestCounts(tasks), {
    prepared: 1, active: 2, completed: 2, failed: 2, total: 7,
  });
  // Removing one completed task decreases both completion and total.
  const afterSingleDelete = tasks.filter((item) => item.taskId !== 'completed-1');
  assert.deepEqual(getDownloadRequestCounts(afterSingleDelete), {
    prepared: 1, active: 2, completed: 1, failed: 2, total: 6,
  });
  // Clearing completed history leaves only unfinished/failed requests.
  const afterClear = tasks.filter((item) => item.status !== 'completed');
  assert.deepEqual(getDownloadRequestCounts(afterClear), {
    prepared: 1, active: 2, completed: 0, failed: 2, total: 5,
  });
  assert.equal(getDownloadRequestCounts([]).total, 0);
});

test('activating a prepared task and replacing a failed retry never double-count requests', () => {
  let tasks = [task('prepared', { taskId: 'segment' }), task('failed', { taskId: 'retry-old' })];
  tasks = upsertDownloadTask(tasks, task('queued', { taskId: 'segment' }));
  tasks = upsertDownloadTask(
    tasks.filter((item) => item.taskId !== 'retry-old'),
    task('running', { taskId: 'retry-new' }),
  );
  assert.deepEqual(getDownloadRequestCounts(tasks), {
    prepared: 0, active: 2, completed: 0, failed: 0, total: 2,
  });
});
