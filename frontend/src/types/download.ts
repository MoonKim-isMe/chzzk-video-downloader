export type DownloadType = 'video' | 'audio';

export type DownloadTaskStatus = 'prepared' | 'queued' | 'running' | 'completed' | 'failed' | 'cancelled';
export type DownloadErrorCode =
  | 'authentication_required'
  | 'partial_data_conflict'
  | 'hls_initialization_fragment_order'
  | 'source_url_missing'
  | 'interrupted'
  | 'hls_download_stalled';

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

export interface DownloadSegment {
  title: string;
  position: number;
  numberFileName?: boolean;
  startSeconds: number;
  endSeconds?: number;
  fileName?: string;
}

export interface StartDownloadRequest {
  segment?: DownloadSegment;
  downloadType?: DownloadType;
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
  segment?: DownloadSegment;
  downloadType?: DownloadType;
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
  logPath?: string;
  error?: string;
  errorCode?: DownloadErrorCode;
  queuedAt: string;
  startedAt?: string;
  finishedAt?: string;
}
