export interface Channel {
  channelId: string;
  channelName: string;
  channelImageUrl: string;
  channelDescription: string;
  followerCount: number;
  verifiedMark: boolean;
  openLive: boolean;
  channelUrl: string;
}

export interface ChannelSearchResult {
  channels: Channel[];
  nextOffset: number;
  hasNext: boolean;
}
