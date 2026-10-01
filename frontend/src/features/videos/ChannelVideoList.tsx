import { Alert, Button, Card, Empty, Image, Skeleton, Space, Tag, Typography } from 'antd';
import { useCallback, useEffect, useRef, useState } from 'react';

import { getChannelVideos } from '../../lib/backend';
import type { Channel } from '../../types/channel';
import type { DownloadTaskStatus } from '../../types/download';
import type { Video, VideoListResult } from '../../types/video';

const { Paragraph, Text, Title } = Typography;
const countFormatter = new Intl.NumberFormat('ko-KR');
const PAGE_SIZE = 24;

interface ChannelVideoListProps {
  channel: Channel;
  activeDownloadStatusByVideoNo: Map<number, DownloadTaskStatus>;
  onQueueVideo: (video: Video) => Promise<void>;
}

interface FailedRequest {
  page: number;
  append: boolean;
}

const emptyResult = (): VideoListResult => ({
  videos: [],
  page: 0,
  size: PAGE_SIZE,
  totalCount: 0,
  totalPages: 0,
  hasNext: false,
  nextPage: 0,
});

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

function mergeVideos(current: Video[], incoming: Video[]) {
  const byVideoNo = new Map<number, Video>();

  current.forEach((video) => byVideoNo.set(video.videoNo, video));
  incoming.forEach((video) => byVideoNo.set(video.videoNo, video));

  return [...byVideoNo.values()];
}

interface VideoCardProps {
  video: Video;
  downloadStatus?: DownloadTaskStatus;
  onQueue: (video: Video) => Promise<void>;
}

function VideoCard({ video, downloadStatus, onQueue }: VideoCardProps) {
  const [queueing, setQueueing] = useState(false);
  const active = downloadStatus === 'queued' || downloadStatus === 'running';

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
      <div className="overflow-hidden rounded-lg bg-slate-950">
        {video.thumbnailImageUrl ? (
          <Image
            src={video.thumbnailImageUrl}
            alt={video.videoTitle}
            preview={false}
            className="aspect-video !w-full object-cover"
          />
        ) : (
          <div className="flex aspect-video items-center justify-center">
            <Text className="!text-slate-600">썸네일 없음</Text>
          </div>
        )}
      </div>

      <div className="mt-3 min-w-0">
        <Space size={6} wrap>
          {video.videoType && <Tag>{video.videoType}</Tag>}
          {video.adult && <Tag color="red">성인</Tag>}
          {video.videoCategoryValue && <Tag color="blue">{video.videoCategoryValue}</Tag>}
          {downloadStatus === 'running' && <Tag color="processing">다운로드 중</Tag>}
          {downloadStatus === 'queued' && <Tag>Queue 대기 중</Tag>}
        </Space>

        <Title level={5} ellipsis={{ rows: 2 }} className="!mb-1 !mt-2 !text-slate-100">
          {video.videoTitle}
        </Title>

        <Text className="block !text-xs !text-slate-500">{video.publishDate || '게시일 정보 없음'}</Text>

        <div className="mt-3 flex flex-wrap gap-x-3 gap-y-1 text-xs">
          <Text className="!text-slate-400">재생시간 {formatDuration(video.duration)}</Text>
          <Text className="!text-slate-400">조회 {countFormatter.format(video.readCount)}</Text>
        </div>

        {video.tags.length > 0 && (
          <Paragraph ellipsis={{ rows: 1 }} className="!mb-0 !mt-2 !text-xs !text-slate-500">
            {video.tags.map((tag) => `#${tag}`).join(' ')}
          </Paragraph>
        )}

        <Button
          block
          className="mt-3"
          type={active ? 'default' : 'primary'}
          disabled={active}
          loading={queueing}
          onClick={() => void handleQueue()}
        >
          {downloadStatus === 'running'
            ? '다운로드 중'
            : downloadStatus === 'queued'
              ? 'Queue 대기 중'
              : 'Queue에 추가'}
        </Button>
      </div>
    </Card>
  );
}

function ChannelVideoList({
  channel,
  activeDownloadStatusByVideoNo,
  onQueueVideo,
}: ChannelVideoListProps) {
  const [result, setResult] = useState<VideoListResult>(emptyResult);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState('');
  const [failedRequest, setFailedRequest] = useState<FailedRequest>();
  const requestSequence = useRef(0);

  const fetchPage = useCallback(
    async (page: number, append: boolean) => {
      const requestID = ++requestSequence.current;

      append ? setLoadingMore(true) : setLoading(true);
      setError('');
      setFailedRequest(undefined);

      try {
        const next = await getChannelVideos(channel.channelId, page, PAGE_SIZE);
        if (requestID !== requestSequence.current) {
          return;
        }

        setResult((current) => ({
          ...next,
          videos: append ? mergeVideos(current.videos, next.videos) : mergeVideos([], next.videos),
        }));
      } catch (cause) {
        if (requestID !== requestSequence.current) {
          return;
        }

        setError(cause instanceof Error ? cause.message : String(cause));
        setFailedRequest({ page, append });
      } finally {
        if (requestID === requestSequence.current) {
          setLoading(false);
          setLoadingMore(false);
        }
      }
    },
    [channel.channelId],
  );

  useEffect(() => {
    setResult(emptyResult());
    setError('');
    setFailedRequest(undefined);
    void fetchPage(0, false);

    return () => {
      requestSequence.current += 1;
    };
  }, [fetchPage]);

  const loadMore = () => {
    if (!result.hasNext || loadingMore) {
      return;
    }
    void fetchPage(result.nextPage, true);
  };

  const retry = () => {
    if (!failedRequest) {
      return;
    }
    void fetchPage(failedRequest.page, failedRequest.append);
  };

  return (
    <Card className="border-slate-800 bg-slate-900/80">
      <div className="mb-5 flex flex-wrap items-end justify-between gap-3">
        <div>
          <Text className="!text-xs !font-semibold !uppercase !tracking-wider !text-slate-500">
            선택 채널 VOD
          </Text>
          <Title level={3} className="!mb-1 !mt-1 !text-slate-100">
            {channel.channelName}
          </Title>
          <Text className="!text-slate-400">
            {loading
              ? 'VOD 목록을 불러오는 중입니다.'
              : `전체 ${countFormatter.format(result.totalCount)}개 · ${countFormatter.format(result.videos.length)}개 불러옴`}
          </Text>
        </div>
        {!loading && result.totalPages > 0 && (
          <Text className="!text-xs !text-slate-500">
            {result.page + 1} / {result.totalPages} 페이지까지 조회
          </Text>
        )}
      </div>

      {error && (
        <Alert
          className="mb-4"
          type="error"
          showIcon
          message={error}
          action={
            <Button size="small" onClick={retry}>
              다시 시도
            </Button>
          }
        />
      )}

      {loading ? (
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {Array.from({ length: 6 }, (_, index) => (
            <Card key={index} className="border-slate-800 bg-slate-900/60">
              <Skeleton active paragraph={{ rows: 3 }} />
            </Card>
          ))}
        </div>
      ) : result.videos.length === 0 && !error ? (
        <Empty description="표시할 VOD가 없습니다." />
      ) : (
        <>
          <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
            {result.videos.map((video) => (
              <VideoCard
                key={video.videoNo}
                video={video}
                downloadStatus={activeDownloadStatusByVideoNo.get(video.videoNo)}
                onQueue={onQueueVideo}
              />
            ))}
          </div>

          {result.hasNext && (
            <Button block className="mt-5" loading={loadingMore} disabled={Boolean(error)} onClick={loadMore}>
              더 보기
            </Button>
          )}
        </>
      )}
    </Card>
  );
}

export default ChannelVideoList;
