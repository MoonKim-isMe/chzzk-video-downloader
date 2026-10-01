export interface VideoChannel {
  channelId: string;
  channelName: string;
  channelImageUrl: string;
  verifiedMark: boolean;
}

export interface Video {
  videoNo: number;
  videoId: string;
  videoTitle: string;
  videoType: string;
  publishDate: string;
  publishDateAt: number;
  thumbnailImageUrl: string;
  trailerUrl: string;
  duration: number;
  readCount: number;
  livePv: number;
  categoryType: string;
  videoCategory: string;
  videoCategoryValue: string;
  exposure: boolean;
  adult: boolean;
  clipActive: boolean;
  commentActive: boolean;
  chapterActive: boolean;
  tags: string[];
  channel: VideoChannel;
  videoUrl: string;
}

export interface VideoListResult {
  videos: Video[];
  page: number;
  size: number;
  totalCount: number;
  totalPages: number;
  hasNext: boolean;
  nextPage: number;
}
