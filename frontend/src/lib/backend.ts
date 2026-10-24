import type { AuthenticationSettings } from '../types/authentication';
import type { Channel, ChannelSearchResult } from '../types/channel';
import type { LiveClip, LiveSession, LiveState, LiveStatus } from '../types/live';
import type {
  DownloadTask,
  StartDownloadRequest,
  ToolchainStatus,
} from '../types/download';
import type { AppSettings } from '../types/settings';
import type { UpdateInfo } from '../types/update';
import type {
  VideoDeepInspectionCPUProfile,
  VideoInspectionResult,
  VideoRepairFileInfo,
  VideoRepairPlan,
  VideoRepairResult,
} from '../types/videoRepair';
import type { Video, VideoListResult } from '../types/video';

interface BackendApp {
  SearchChannels(keyword: string, offset: number, size: number): Promise<ChannelSearchResult>;
  GetChannelLiveStatus(channelId: string): Promise<LiveStatus>;
  GetLiveRecordingState(): Promise<LiveState>;
  ConfigureLiveRecording(pollSeconds: number, maxConcurrent: number): Promise<LiveState>;
  SetLiveAutoRecording(channelId: string, channelName: string, enabled: boolean): Promise<LiveState>;
  RemoveLiveAutoRecording(channelId: string): Promise<LiveState>;
  StartLiveRecording(channelId: string): Promise<LiveSession>;
  StopLiveRecording(sessionId: string): Promise<boolean>;
  DeleteLiveRecordingHistory(sessionId: string): Promise<LiveState>;
  ClearLiveRecordingHistory(): Promise<LiveState>;
  ExtractLiveClip(sessionId: string, title: string, startSeconds: number, endSeconds: number, exact: boolean): Promise<LiveClip>;
  ResolveVideoURL(rawURL: string): Promise<Video>;
  GetSavedChannels(): Promise<Channel[]>;
  SaveChannel(channel: Channel): Promise<Channel[]>;
  RemoveSavedChannel(channelId: string): Promise<Channel[]>;
  GetChannelVideos(channelId: string, page: number, size: number): Promise<VideoListResult>;
  GetDownloadToolchainStatus(): Promise<ToolchainStatus>;
  GetDefaultDownloadDir(): Promise<string>;
  SelectDownloadDirectory(currentDirectory: string): Promise<string>;
  OpenDownloadDirectory(directory: string): Promise<void>;
  SelectVideoRepairFile(currentFile: string): Promise<VideoRepairFileInfo>;
  StartQuickVideoInspection(path: string): Promise<VideoInspectionResult>;
  StartDeepVideoInspection(path: string, cpuProfile: VideoDeepInspectionCPUProfile): Promise<VideoInspectionResult>;
  CancelVideoInspection(): Promise<boolean>;
  CreateVideoRepairPlan(result: VideoInspectionResult): Promise<VideoRepairPlan>;
  CreateVideoCompatibilityPlan(result: VideoInspectionResult): Promise<VideoRepairPlan>;
  StartVideoRepair(plan: VideoRepairPlan): Promise<VideoRepairResult>;
  CancelVideoRepair(): Promise<boolean>;
  RevealVideoRepairFile(path: string): Promise<void>;
  GetAuthenticationSettings(): Promise<AuthenticationSettings>;
  UpdateAuthenticationSettings(settings: AuthenticationSettings): Promise<AuthenticationSettings>;
  SelectAuthenticationCookiesFile(currentFile: string): Promise<string>;
  GetSettings(): Promise<AppSettings>;
  UpdateSettings(settings: AppSettings): Promise<AppSettings>;
  CheckForUpdates(): Promise<UpdateInfo>;
  OpenReleasePage(rawURL: string): Promise<void>;
  StartDownload(request: StartDownloadRequest): Promise<DownloadTask>;
  StartPreparedSegment(taskId: string): Promise<DownloadTask>;
  GetDownloadTasks(): Promise<DownloadTask[]>;
  CancelDownload(taskId: string): Promise<boolean>;
  RecoverDownload(taskId: string): Promise<DownloadTask>;
  DeleteDownloadTask(taskId: string): Promise<void>;
  ClearCompletedDownloadTasks(): Promise<string[]>;
  OpenDownloadLog(taskId: string): Promise<void>;
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

export const getChannelLiveStatus = (channelId: string) => app().GetChannelLiveStatus(channelId);
export const getLiveRecordingState = () => app().GetLiveRecordingState();
export const configureLiveRecording = (pollSeconds: number, maxConcurrent: number) => app().ConfigureLiveRecording(pollSeconds, maxConcurrent);
export const setLiveAutoRecording = (channelId: string, channelName: string, enabled: boolean) => app().SetLiveAutoRecording(channelId, channelName, enabled);
export const removeLiveAutoRecording = (channelId: string) => app().RemoveLiveAutoRecording(channelId);
export const startLiveRecording = (channelId: string) => app().StartLiveRecording(channelId);
export const stopLiveRecording = (sessionId: string) => app().StopLiveRecording(sessionId);
export const deleteLiveRecordingHistory = (sessionId: string) => app().DeleteLiveRecordingHistory(sessionId);
export const clearLiveRecordingHistory = () => app().ClearLiveRecordingHistory();
export const extractLiveClip = (sessionId: string, title: string, startSeconds: number, endSeconds: number, exact: boolean) => app().ExtractLiveClip(sessionId, title, startSeconds, endSeconds, exact);

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

export const openDownloadDirectory = (directory: string) => app().OpenDownloadDirectory(directory);

export const selectVideoRepairFile = (currentFile = '') =>
  app().SelectVideoRepairFile(currentFile);

export const startQuickVideoInspection = (path: string) =>
  app().StartQuickVideoInspection(path);

export const startDeepVideoInspection = (path: string, cpuProfile: VideoDeepInspectionCPUProfile) =>
  app().StartDeepVideoInspection(path, cpuProfile);

export const cancelVideoInspection = () => app().CancelVideoInspection();

export const createVideoRepairPlan = (result: VideoInspectionResult) =>
  app().CreateVideoRepairPlan(result);

export const createVideoCompatibilityPlan = (result: VideoInspectionResult) =>
  app().CreateVideoCompatibilityPlan(result);

export const startVideoRepair = (plan: VideoRepairPlan) => app().StartVideoRepair(plan);

export const cancelVideoRepair = () => app().CancelVideoRepair();

export const revealVideoRepairFile = (path: string) => app().RevealVideoRepairFile(path);

export const getAuthenticationSettings = () => app().GetAuthenticationSettings();

export const updateAuthenticationSettings = (settings: AuthenticationSettings) =>
  app().UpdateAuthenticationSettings(settings);

export const selectAuthenticationCookiesFile = (currentFile: string) =>
  app().SelectAuthenticationCookiesFile(currentFile);

export const getSettings = () => app().GetSettings();

export const updateSettings = (settings: AppSettings) => app().UpdateSettings(settings);

export const checkForUpdates = () => app().CheckForUpdates();

export const openReleasePage = (rawURL: string) => app().OpenReleasePage(rawURL);

export const startDownload = (request: StartDownloadRequest) => app().StartDownload(request);

export const startPreparedSegment = (taskId: string) => app().StartPreparedSegment(taskId);

export const getDownloadTasks = () => app().GetDownloadTasks();

export const cancelDownload = (taskId: string) => app().CancelDownload(taskId);

export const recoverDownload = (taskId: string) => app().RecoverDownload(taskId);

export const deleteDownloadTask = (taskId: string) => app().DeleteDownloadTask(taskId);

export const clearCompletedDownloadTasks = () => app().ClearCompletedDownloadTasks();

export const openDownloadLog = (taskId: string) => app().OpenDownloadLog(taskId);

export const openDownloadFolder = (taskId: string) => app().OpenDownloadFolder(taskId);
