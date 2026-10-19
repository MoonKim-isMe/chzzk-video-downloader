import { Card, Image, Space, Tag, Typography } from 'antd';
import { useEffect, useState } from 'react';

import type { DownloadTaskStatus, DownloadType } from '../../types/download';
import DownloadButtonGroup from './DownloadButtonGroup';
import type { Video } from '../../types/video';

const { Text, Title } = Typography;
const countFormatter = new Intl.NumberFormat('ko-KR');

function formatDuration(seconds: number) {
  const safeSeconds = Math.max(0, Math.floor(seconds || 0));
  const hours = Math.floor(safeSeconds / 3600);
  const minutes = Math.floor((safeSeconds % 3600) / 60);
  const remainSeconds = safeSeconds % 60;

  if (hours > 0) {
    return [hours, minutes, remainSeconds].map((value) => String(value).padStart(2, '0')).join(':');
  }
  return [minutes, remainSeconds].map((value) => String(value).padStart(2, '0')).join(':');
}

interface VideoCardProps {
  video: Video;
  videoDownloadStatus?: DownloadTaskStatus;
  audioDownloadStatus?: DownloadTaskStatus;
  onQueue: (video: Video, downloadType?: DownloadType) => Promise<void>;
  onOpenSegments: (video: Video) => void;
}

function VideoCard({ video, videoDownloadStatus, audioDownloadStatus, onQueue, onOpenSegments }: VideoCardProps) {
  const [thumbnailFailed, setThumbnailFailed] = useState(false);
  const active = videoDownloadStatus === 'queued' || videoDownloadStatus === 'running';
  const audioActive = audioDownloadStatus === 'queued' || audioDownloadStatus === 'running';

  useEffect(() => {
    setThumbnailFailed(false);
  }, [video.thumbnailImageUrl]);

  return (
    <Card
      size="small"
      className={`video-card h-full overflow-hidden bg-slate-900/80 ${
        active || audioActive ? 'border-emerald-500/70' : 'border-slate-800'
      }`}
      styles={{ body: { padding: 12, height: '100%' } }}
    >
      <div className="video-card-content">
        <div className="video-thumbnail">
        {video.thumbnailImageUrl && !thumbnailFailed ? (
          <Image
            src={video.thumbnailImageUrl}
            alt={video.videoTitle}
            preview={false}
            className="aspect-video !w-full object-cover"
            onError={() => setThumbnailFailed(true)}
          />
        ) : (
          <div className="video-thumbnail-placeholder">
            <Text className="!text-center !text-[13px] !text-slate-500">
              썸네일이 없거나 불러오지 못했습니다
            </Text>
          </div>
        )}
        </div>

        <div className="mt-3 min-w-0">
          <Space size={6} wrap>
          {video.videoType && <Tag>{video.videoType}</Tag>}
          {video.adult && <Tag color="red">성인</Tag>}
          {video.videoCategoryValue && <Tag color="blue">{video.videoCategoryValue}</Tag>}
          {videoDownloadStatus === 'running' && <Tag color="processing">동영상 다운로드 중</Tag>}
          {videoDownloadStatus === 'queued' && <Tag>동영상 대기 중</Tag>}
          {audioDownloadStatus === 'running' && <Tag color="purple">MP3 다운로드 중</Tag>}
          {audioDownloadStatus === 'queued' && <Tag>MP3 대기 중</Tag>}
        </Space>

          <Title
            level={5}
            ellipsis={{ rows: 2 }}
            className="video-card-title !mb-1 !mt-2 !text-slate-100"
          >
            {video.videoTitle}
          </Title>

          {video.channel.channelName && (
            <Text className="block !text-[13px] !text-slate-400">{video.channel.channelName}</Text>
          )}
          <Text className="mt-1 block !text-[13px] !text-slate-500">
            {video.publishDate || '게시일 정보 없음'}
          </Text>
        </div>

        <div className="video-card-footer">
          <div className="video-card-metrics">
            <Text className="!text-slate-400">재생시간 {formatDuration(video.duration)}</Text>
            <Text className="!text-slate-400">조회 {countFormatter.format(video.readCount)}</Text>
          </div>

          <div className="video-tag-chips" aria-label="VOD 태그">
            {video.tags.map((tag) => (
              <span key={tag} className="video-tag-chip">
                #{tag}
              </span>
            ))}
          </div>

          <DownloadButtonGroup
            className="video-card-action"
            videoStatus={videoDownloadStatus}
            audioStatus={audioDownloadStatus}
            onQueue={(downloadType) => onQueue(video, downloadType)}
            onOpenSegments={() => onOpenSegments(video)}
          />
        </div>
      </div>
    </Card>
  );
}

export default VideoCard;
