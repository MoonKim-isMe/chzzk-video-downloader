import type { DownloadTask } from '../types/download';
import type { VideoInspectionProgress, VideoRepairProgress } from '../types/videoRepair';

export const DOWNLOAD_STATE_EVENT = 'download:state';
export const VIDEO_INSPECTION_PROGRESS_EVENT = 'video-repair:inspection-progress';
export const VIDEO_REPAIR_PROGRESS_EVENT = 'video-repair:repair-progress';

type RuntimeCallback = (...data: unknown[]) => void;

type WailsRuntimeWindow = Window & {
  runtime?: {
    EventsOn?: (eventName: string, callback: RuntimeCallback) => () => void;
  };
};

export function onDownloadState(handler: (task: DownloadTask) => void) {
  const eventsOn = (window as WailsRuntimeWindow).runtime?.EventsOn;
  if (!eventsOn) {
    return () => undefined;
  }

  return eventsOn(DOWNLOAD_STATE_EVENT, (...data: unknown[]) => {
    const payload = data[0];
    if (payload && typeof payload === 'object') {
      handler(payload as DownloadTask);
    }
  });
}

export function onVideoInspectionProgress(handler: (progress: VideoInspectionProgress) => void) {
  const eventsOn = (window as WailsRuntimeWindow).runtime?.EventsOn;
  if (!eventsOn) {
    return () => undefined;
  }

  return eventsOn(VIDEO_INSPECTION_PROGRESS_EVENT, (...data: unknown[]) => {
    const payload = data[0];
    if (payload && typeof payload === 'object') {
      handler(payload as VideoInspectionProgress);
    }
  });
}

export function onVideoRepairProgress(handler: (progress: VideoRepairProgress) => void) {
  const eventsOn = (window as WailsRuntimeWindow).runtime?.EventsOn;
  if (!eventsOn) {
    return () => undefined;
  }

  return eventsOn(VIDEO_REPAIR_PROGRESS_EVENT, (...data: unknown[]) => {
    const payload = data[0];
    if (payload && typeof payload === 'object') {
      handler(payload as VideoRepairProgress);
    }
  });
}
