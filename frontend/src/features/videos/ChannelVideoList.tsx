import { Alert, Avatar, Button, Card, Empty, Skeleton, Typography } from 'antd';
import { useCallback, useEffect, useRef, useState } from 'react';

import { getChannelVideos } from '../../lib/backend';
import type { Channel } from '../../types/channel';
import type { DownloadTaskStatus } from '../../types/download';
import type { Video, VideoListResult } from '../../types/video';
import VideoCard from './VideoCard';

const { Text, Title } = Typography;
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

function mergeVideos(current: Video[], incoming: Video[]) {
  const byVideoNo = new Map<number, Video>();
  current.forEach((video) => byVideoNo.set(video.videoNo, video));
  incoming.forEach((video) => byVideoNo.set(video.videoNo, video));
  return [...byVideoNo.values()];
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

  const retry = () => {
    if (failedRequest) {
      void fetchPage(failedRequest.page, failedRequest.append);
    }
  };

  return (
    <Card bordered={false} className="app-panel channel-vod-panel">
      <div className="channel-vod-header">
        <div className="channel-vod-identity">
          <Avatar size={46} src={channel.channelImageUrl || undefined}>
            {channel.channelName.slice(0, 1)}
          </Avatar>
          <div className="min-w-0">
            <Text className="app-eyebrow">VOD</Text>
            <Title level={4} ellipsis className="app-section-title !mb-0 !mt-1">
              {channel.channelName}
            </Title>
          </div>
        </div>

        <div className="channel-vod-meta">
          <Text className="app-muted !text-xs">
            {loading
              ? '불러오는 중'
              : `전체 ${countFormatter.format(result.totalCount)}개 · ${countFormatter.format(result.videos.length)}개 표시`}
          </Text>
          {!loading && result.totalPages > 0 && (
            <Text className="app-muted !text-xs">
              {result.page + 1} / {result.totalPages} 페이지
            </Text>
          )}
        </div>
      </div>

      {error && (
        <div className="channel-vod-alert">
          <Alert
            type="error"
            showIcon
            message={error}
            action={
              <Button size="small" onClick={retry}>
                다시 시도
              </Button>
            }
          />
        </div>
      )}

      <div className="channel-vod-scroll">
        {loading ? (
          <div className="channel-vod-grid">
            {Array.from({ length: 6 }, (_, index) => (
              <Card key={index} className="border-slate-800 bg-slate-900/60">
                <Skeleton active paragraph={{ rows: 3 }} />
              </Card>
            ))}
          </div>
        ) : result.videos.length === 0 && !error ? (
          <div className="channel-vod-empty-content">
            <Empty description="표시할 VOD가 없습니다." />
          </div>
        ) : (
          <>
            <div className="channel-vod-grid">
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
              <Button
                block
                className="mt-4"
                loading={loadingMore}
                disabled={Boolean(error)}
                onClick={() => void fetchPage(result.nextPage, true)}
              >
                더 보기
              </Button>
            )}
          </>
        )}
      </div>
    </Card>
  );
}

export default ChannelVideoList;
