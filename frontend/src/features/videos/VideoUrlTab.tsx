import { Alert, Avatar, Card, Empty, Image, Input, Space, Tag, Typography } from 'antd';
import { useEffect, useRef, useState } from 'react';

import { resolveVideoURL } from '../../lib/backend';
import type { DownloadTaskStatus, DownloadType } from '../../types/download';
import DownloadButtonGroup from './DownloadButtonGroup';
import SegmentDownloadEditor from './SegmentDownloadEditor';
import type { SegmentRange } from './segmentRanges';
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
  activeDownloadStatusByVideoNo: Map<string, DownloadTaskStatus>;
  onQueueVideo: (video: Video, downloadType?: DownloadType) => Promise<void>;
  segmentRequest?: { video: Video; sequence: number };
  onQueueSegments: (video: Video, segments: SegmentRange[], downloadType: DownloadType, numberFileName: boolean) => Promise<void>;
}

function VideoUrlTab({ activeDownloadStatusByVideoNo, onQueueVideo, segmentRequest, onQueueSegments }: VideoUrlTabProps) {
  const [url, setURL] = useState('');
  const [video, setVideo] = useState<Video>();
  const [loading, setLoading] = useState(false);
  const [resolved, setResolved] = useState(false);
  const [thumbnailFailed, setThumbnailFailed] = useState(false);
  const [error, setError] = useState('');
  const [segmentsOpen, setSegmentsOpen] = useState(false);
  const requestSequence = useRef(0);

  useEffect(() => {
    if (!segmentRequest) return;
    requestSequence.current += 1;
    setURL(segmentRequest.video.videoUrl);
    setVideo(segmentRequest.video);
    setResolved(true);
    setLoading(false);
    setError('');
    setSegmentsOpen(true);
  }, [segmentRequest]);

  useEffect(() => () => { requestSequence.current += 1; }, []);

  const videoDownloadStatus = video
    ? activeDownloadStatusByVideoNo.get(`${video.videoNo}:video`)
    : undefined;
  const audioDownloadStatus = video
    ? activeDownloadStatusByVideoNo.get(`${video.videoNo}:audio`)
    : undefined;

  useEffect(() => {
    setThumbnailFailed(false);
  }, [video?.thumbnailImageUrl]);

  const resolve = async () => {
    const requestId = ++requestSequence.current;
    setSegmentsOpen(false);
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
      if (requestId !== requestSequence.current) return;
      setVideo(nextVideo);
      setResolved(true);
    } catch (cause) {
      if (requestId !== requestSequence.current) return;
      setError(cause instanceof Error ? cause.message : String(cause));
      setResolved(true);
    } finally {
      if (requestId === requestSequence.current) setLoading(false);
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
          <>
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
                  <Text className="!text-center !text-[13px] !text-slate-500">
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
                  {videoDownloadStatus === 'running' && <Tag color="processing">동영상 다운로드 중</Tag>}
                  {videoDownloadStatus === 'queued' && <Tag>동영상 대기 중</Tag>}
                  {audioDownloadStatus === 'running' && <Tag color="purple">MP3 다운로드 중</Tag>}
                  {audioDownloadStatus === 'queued' && <Tag>MP3 대기 중</Tag>}
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
                    <Text className="app-muted block !text-[13px]">
                      {video.publishDate || '게시일 정보 없음'}
                    </Text>
                  </div>
                </div>
              </div>

              <div className="url-video-footer">
                <div className="url-video-metrics">
                  <div>
                    <Text className="app-muted block !text-[13px]">재생시간</Text>
                    <Text strong>{formatDuration(video.duration)}</Text>
                  </div>
                  <div>
                    <Text className="app-muted block !text-[13px]">조회</Text>
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

                <DownloadButtonGroup
                  size="large"
                  videoStatus={videoDownloadStatus}
                  audioStatus={audioDownloadStatus}
                  onQueue={(downloadType) => onQueueVideo(video, downloadType)}
                  onOpenSegments={() => setSegmentsOpen(true)}
                />
              </div>
            </div>
          </section>
          {segmentsOpen && (
            <SegmentDownloadEditor
              key={video.videoNo}
              duration={video.duration}
              onRegister={(segments, downloadType, numberFileName) => onQueueSegments(video, segments, downloadType, numberFileName)}
            />
          )}
          </>
        )}
      </div>
    </Card>
  );
}

export default VideoUrlTab;
