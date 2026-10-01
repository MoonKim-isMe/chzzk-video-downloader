import type { Channel, ChannelSearchResult } from '../types/channel';

interface BackendApp {
  SearchChannels(keyword: string, offset: number, size: number): Promise<ChannelSearchResult>;
  ResolveChannelURL(rawURL: string): Promise<Channel>;
  GetSavedChannels(): Promise<Channel[]>;
  SaveChannel(channel: Channel): Promise<Channel[]>;
  RemoveSavedChannel(channelId: string): Promise<Channel[]>;
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

export const resolveChannelURL = (rawURL: string) => app().ResolveChannelURL(rawURL);

export const getSavedChannels = () => app().GetSavedChannels();

export const saveChannel = (channel: Channel) => app().SaveChannel(channel);

export const removeSavedChannel = (channelId: string) => app().RemoveSavedChannel(channelId);
