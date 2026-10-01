import type { DownloadTask } from '../types/download';

export const DOWNLOAD_STATE_EVENT = 'download:state';

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
