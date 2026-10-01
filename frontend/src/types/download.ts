export type DownloadTaskStatus = 'queued' | 'running' | 'completed' | 'failed' | 'cancelled';
export type DownloadErrorCode = 'authentication_required' | 'partial_data_conflict';

export interface ToolStatus {
  name: string;
  path: string;
  version: string;
  source: string;
  found: boolean;
  available: boolean;
  error?: string;
}

export interface ToolchainStatus {
  ytDlp: ToolStatus;
  ffmpeg: ToolStatus;
  ffprobe: ToolStatus;
  downloadReady: boolean;
  mergeReady: boolean;
}

export interface DownloadProgress {
  status: string;
  percent: number;
  downloadedBytes: number;
  totalBytes: number;
  totalBytesEstimated: boolean;
  speedBytesPerSecond: number;
  etaSeconds: number;
}

export interface StartDownloadRequest {
  videoNo: number;
  videoTitle: string;
  channelName: string;
  thumbnailImageUrl: string;
  url: string;
  outputDir: string;
  formatSelector?: string;
  outputTemplate?: string;
}

export interface DownloadTask {
  taskId: string;
  videoNo: number;
  videoTitle: string;
  channelName: string;
  thumbnailImageUrl: string;
  url: string;
  outputDir: string;
  status: DownloadTaskStatus;
  progress: DownloadProgress;
  finalPath?: string;
  error?: string;
  errorCode?: DownloadErrorCode;
  queuedAt: string;
  startedAt?: string;
  finishedAt?: string;
}
