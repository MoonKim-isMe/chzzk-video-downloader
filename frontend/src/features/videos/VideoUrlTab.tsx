import { Alert, Avatar, Button, Card, Empty, Image, Input, Space, Tag, Typography } from 'antd';
import { useEffect, useState } from 'react';

import { resolveVideoURL } from '../../lib/backend';
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
    return [hours, minutes, remainSeconds]
      .map((value) => String(value).padStart(2, '0'))
      .join(':');
  }
  return [minutes, remainSeconds]
    .map((value) => String(value).padStart(2, '0'))
    .join(':');
}

interface VideoUrlTabProps {
  activeDownloadStatusByVideoNo: Map<number, DownloadTaskStatus>;
  onQueueVideo: (video: Video) => Promise<void>;
}

function VideoUrlTab({ activeDownloadStatusByVideoNo, onQueueVideo }: VideoUrlTabProps) {
  const [url, setURL] = useState('');
  const [video, setVideo] = useState<Video>();
  const [loading, setLoading] = useState(false);
  const [queueing, setQueueing] = useState(false);
  const [resolved, setResolved] = useState(false);
  const [thumbnailFailed, setThumbnailFailed] = useState(false);
  const [error, setError] = useState('');

  const downloadStatus = video
    ? activeDownloadStatusByVideoNo.get(video.videoNo)
    : undefined;
  const active = downloadStatus === 'queued' || downloadStatus === 'running';

  useEffect(() => {
    setThumbnailFailed(false);
  }, [video?.thumbnailImageUrl]);

  const resolve = async () => {
    const normalizedURL = url.trim();
    if (!normalizedURL) {
      setError('치지직 VOD URL을 입력해 주세요.');
      setVideo(undefined);
      setResolved(true);
      return;
    }

    setLoading(true);
    setError('');
    setVideo(undefined);
    try {
      const nextVideo = await resolveVideoURL(normalizedURL);
      setVideo(nextVideo);
      setResolved(true);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
      setResolved(true);
    } finally {
      setLoading(false);
    }
  };

  const handleQueue = async () => {
    if (!video || active || queueing) {
      return;
    }

    setQueueing(true);
    try {
      await onQueueVideo(video);
    } finally {
      setQueueing(false);
    }
  };

  return (
    <Card bordered={false} className="app-panel url-workspace-panel">
      <div className="url-search-header">
        <Input.Search
          value={url}
          onChange={(event) => setURL(event.target.value)}
          onSearch={() => void resolve()}
          enterButton="검색"
          placeholder="https://chzzk.naver.com/video/{videoNo}"
          size="large"
          loading={loading}
        />
        {error && <Alert type="error" showIcon message={error} />}
      </div>

      <div className="url-result-scroll">
        {!video && !loading && (
          <div className="url-result-empty">
            <Empty
              description={
                resolved && !error
                  ? 'VOD 정보를 찾을 수 없습니다.'
                  : '치지직 VOD URL을 입력해 주세요.'
              }
            />
          </div>
        )}

        {loading && (
          <div className="url-result-loading">
            <div className="url-result-loading-thumbnail" />
            <div className="url-result-loading-info">
              <div className="url-result-loading-line is-short" />
              <div className="url-result-loading-line is-title" />
              <div className="url-result-loading-line" />
              <div className="url-result-loading-line is-half" />
            </div>
          </div>
        )}

        {video && (
          <section className="url-video-result">
            <div className="url-video-thumbnail">
              {video.thumbnailImageUrl && !thumbnailFailed ? (
                <Image
                  src={video.thumbnailImageUrl}
                  alt={video.videoTitle}
                  preview={false}
                  className="!h-full !w-full object-cover"
                  onError={() => setThumbnailFailed(true)}
                />
              ) : (
                <div className="url-video-thumbnail-placeholder">
                  <Text className="!text-center !text-xs !text-slate-500">
                    썸네일이 없거나 불러오지 못했습니다
                  </Text>
                </div>
              )}
            </div>

            <div className="url-video-info">
              <div>
                <Space size={6} wrap>
                  {video.videoType && <Tag>{video.videoType}</Tag>}
                  {video.adult && <Tag color="red">성인</Tag>}
                  {video.videoCategoryValue && (
                    <Tag color="blue">{video.videoCategoryValue}</Tag>
                  )}
                  {downloadStatus === 'running' && (
                    <Tag color="processing">다운로드 중</Tag>
                  )}
                  {downloadStatus === 'queued' && <Tag>다운로드 대기 중</Tag>}
                </Space>

                <Title level={3} className="url-video-title !mb-0 !mt-3">
                  {video.videoTitle}
                </Title>

                <div className="url-video-channel">
                  <Avatar size={34} src={video.channel.channelImageUrl || undefined}>
                    {video.channel.channelName.slice(0, 1)}
                  </Avatar>
                  <div className="min-w-0">
                    <Text strong ellipsis className="block">
                      {video.channel.channelName}
                    </Text>
                    <Text className="app-muted block !text-xs">
                      {video.publishDate || '게시일 정보 없음'}
                    </Text>
                  </div>
                </div>
              </div>

              <div className="url-video-footer">
                <div className="url-video-metrics">
                  <div>
                    <Text className="app-muted block !text-[10px]">재생시간</Text>
                    <Text strong>{formatDuration(video.duration)}</Text>
                  </div>
                  <div>
                    <Text className="app-muted block !text-[10px]">조회</Text>
                    <Text strong>{countFormatter.format(video.readCount)}</Text>
                  </div>
                </div>

                <div className="url-video-tags" aria-label="VOD 태그">
                  {video.tags.map((tag) => (
                    <span key={tag} className="video-tag-chip">
                      #{tag}
                    </span>
                  ))}
                </div>

                <Button
                  type={active ? 'default' : 'primary'}
                  size="large"
                  block
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
            </div>
          </section>
        )}
      </div>
    </Card>
  );
}

export default VideoUrlTab;
