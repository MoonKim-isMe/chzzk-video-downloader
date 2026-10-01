export type DownloadResolution = 'best' | '2160p' | '1440p' | '1080p' | '720p';

export type OutputFormat = 'mp4' | 'mkv' | 'webm';

export type ThemeMode = 'light' | 'dark';

export interface AppSettings {
  downloadDir: string;
  resolution: DownloadResolution;
  outputFormat: OutputFormat;
  maxConcurrentDownloads: number;
  theme: ThemeMode;
}
