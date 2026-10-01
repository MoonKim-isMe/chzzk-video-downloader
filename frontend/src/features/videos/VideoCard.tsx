import { Button, Card, Image, Space, Tag, Typography } from 'antd';
import { useEffect, useState } from 'react';

import type { DownloadTaskStatus } from '../../types/download';
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
  downloadStatus?: DownloadTaskStatus;
  onQueue: (video: Video) => Promise<void>;
}

function VideoCard({ video, downloadStatus, onQueue }: VideoCardProps) {
  const [queueing, setQueueing] = useState(false);
  const [thumbnailFailed, setThumbnailFailed] = useState(false);
  const active = downloadStatus === 'queued' || downloadStatus === 'running';

  useEffect(() => {
    setThumbnailFailed(false);
  }, [video.thumbnailImageUrl]);

  const handleQueue = async () => {
    if (active || queueing) {
      return;
    }

    setQueueing(true);
    try {
      await onQueue(video);
    } finally {
      setQueueing(false);
    }
  };

  return (
    <Card
      size="small"
      className={`h-full overflow-hidden bg-slate-900/80 ${
        active ? 'border-emerald-500/70' : 'border-slate-800'
      }`}
      styles={{ body: { padding: 12 } }}
    >
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
            <Text className="!text-center !text-xs !text-slate-500">
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
          {downloadStatus === 'running' && <Tag color="processing">다운로드 중</Tag>}
          {downloadStatus === 'queued' && <Tag>다운로드 대기 중</Tag>}
        </Space>

        <Title level={5} ellipsis={{ rows: 2 }} className="!mb-1 !mt-2 !text-slate-100">
          {video.videoTitle}
        </Title>

        {video.channel.channelName && (
          <Text className="block !text-xs !text-slate-400">{video.channel.channelName}</Text>
        )}
        <Text className="mt-1 block !text-xs !text-slate-500">
          {video.publishDate || '게시일 정보 없음'}
        </Text>

        <div className="mt-3 flex flex-wrap gap-x-3 gap-y-1 text-xs">
          <Text className="!text-slate-400">재생시간 {formatDuration(video.duration)}</Text>
          <Text className="!text-slate-400">조회 {countFormatter.format(video.readCount)}</Text>
        </div>

        {video.tags.length > 0 && (
          <div className="video-tag-chips">
            {video.tags.map((tag) => (
              <span key={tag} className="video-tag-chip">
                #{tag}
              </span>
            ))}
          </div>
        )}

        <Button
          block
          className="video-card-action"
          type={active ? 'default' : 'primary'}
          disabled={active}
          loading={queueing}
          onClick={() => void handleQueue()}
        >
          {downloadStatus === 'running'
            ? '다운로드 중'
            : downloadStatus === 'queued'
              ? '다운로드 대기 중'
              : '다운로드 추가'}
        </Button>
      </div>
    </Card>
  );
}

export default VideoCard;
