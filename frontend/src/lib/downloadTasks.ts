import type { DownloadTask } from '../types/download';

const statusRank: Record<DownloadTask['status'], number> = {
  queued: 0,
  running: 1,
  completed: 2,
  failed: 2,
  cancelled: 2,
};

function finishedAtValue(task: DownloadTask) {
  return task.finishedAt ? Date.parse(task.finishedAt) || 0 : 0;
}

export function pickFresherDownloadTask(current: DownloadTask, incoming: DownloadTask) {
  const currentRank = statusRank[current.status];
  const incomingRank = statusRank[incoming.status];

  if (currentRank !== incomingRank) {
    return incomingRank > currentRank ? incoming : current;
  }

  if (incomingRank === 2) {
    return finishedAtValue(incoming) >= finishedAtValue(current) ? incoming : current;
  }

  if (incoming.status === 'running' && current.status === 'running') {
    if (incoming.progress.downloadedBytes !== current.progress.downloadedBytes) {
      return incoming.progress.downloadedBytes > current.progress.downloadedBytes ? incoming : current;
    }
    if (incoming.progress.percent !== current.progress.percent) {
      return incoming.progress.percent > current.progress.percent ? incoming : current;
    }
  }

  return incoming;
}

export function upsertDownloadTask(tasks: DownloadTask[], incoming: DownloadTask) {
  const index = tasks.findIndex((task) => task.taskId === incoming.taskId);
  if (index < 0) {
    return [...tasks, incoming];
  }

  const next = [...tasks];
  next[index] = pickFresherDownloadTask(tasks[index], incoming);
  return next;
}

export function mergeDownloadTaskSnapshot(current: DownloadTask[], snapshot: DownloadTask[]) {
  const currentById = new Map(current.map((task) => [task.taskId, task]));
  const snapshotIds = new Set(snapshot.map((task) => task.taskId));

  const merged = snapshot.map((task) => {
    const existing = currentById.get(task.taskId);
    return existing ? pickFresherDownloadTask(task, existing) : task;
  });

  current.forEach((task) => {
    if (!snapshotIds.has(task.taskId)) {
      merged.push(task);
    }
  });

  return merged;
}
