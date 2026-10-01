import type { Channel, ChannelSearchResult } from '../types/channel';
import type {
  DownloadTask,
  StartDownloadRequest,
  ToolchainStatus,
} from '../types/download';
import type { AppSettings } from '../types/settings';
import type { Video, VideoListResult } from '../types/video';

interface BackendApp {
  SearchChannels(keyword: string, offset: number, size: number): Promise<ChannelSearchResult>;
  ResolveVideoURL(rawURL: string): Promise<Video>;
  GetSavedChannels(): Promise<Channel[]>;
  SaveChannel(channel: Channel): Promise<Channel[]>;
  RemoveSavedChannel(channelId: string): Promise<Channel[]>;
  GetChannelVideos(channelId: string, page: number, size: number): Promise<VideoListResult>;
  GetDownloadToolchainStatus(): Promise<ToolchainStatus>;
  GetDefaultDownloadDir(): Promise<string>;
  SelectDownloadDirectory(currentDirectory: string): Promise<string>;
  GetSettings(): Promise<AppSettings>;
  UpdateSettings(settings: AppSettings): Promise<AppSettings>;
  StartDownload(request: StartDownloadRequest): Promise<DownloadTask>;
  GetDownloadTasks(): Promise<DownloadTask[]>;
  CancelDownload(taskId: string): Promise<boolean>;
  DeleteDownloadTask(taskId: string): Promise<void>;
  OpenDownloadFolder(taskId: string): Promise<void>;
}

type WailsWindow = Window & {
  go?: {
    main?: {
      App?: BackendApp;
    };
  };
};

function app(): BackendApp {
  const backend = (window as WailsWindow).go?.main?.App;
  if (!backend) {
    throw new Error('Wails 백엔드에 연결할 수 없습니다.');
  }
  return backend;
}

export const searchChannels = (keyword: string, offset = 0, size = 20) =>
  app().SearchChannels(keyword, offset, size);

export const resolveVideoURL = (rawURL: string) => app().ResolveVideoURL(rawURL);

export const getSavedChannels = () => app().GetSavedChannels();

export const saveChannel = (channel: Channel) => app().SaveChannel(channel);

export const removeSavedChannel = (channelId: string) => app().RemoveSavedChannel(channelId);

export const getChannelVideos = (channelId: string, page = 0, size = 24) =>
  app().GetChannelVideos(channelId, page, size);

export const getDownloadToolchainStatus = () => app().GetDownloadToolchainStatus();

export const getDefaultDownloadDir = () => app().GetDefaultDownloadDir();

export const selectDownloadDirectory = (currentDirectory: string) =>
  app().SelectDownloadDirectory(currentDirectory);

export const getSettings = () => app().GetSettings();

export const updateSettings = (settings: AppSettings) => app().UpdateSettings(settings);

export const startDownload = (request: StartDownloadRequest) => app().StartDownload(request);

export const getDownloadTasks = () => app().GetDownloadTasks();

export const cancelDownload = (taskId: string) => app().CancelDownload(taskId);

export const deleteDownloadTask = (taskId: string) => app().DeleteDownloadTask(taskId);

export const openDownloadFolder = (taskId: string) => app().OpenDownloadFolder(taskId);
