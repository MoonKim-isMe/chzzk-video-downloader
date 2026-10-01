import { Alert, Empty, Input, Typography } from 'antd';
import { useState } from 'react';

import { resolveVideoURL } from '../../lib/backend';
import type { DownloadTaskStatus } from '../../types/download';
import type { Video } from '../../types/video';
import VideoCard from './VideoCard';

const { Paragraph, Title } = Typography;

interface VideoUrlTabProps {
  activeDownloadStatusByVideoNo: Map<number, DownloadTaskStatus>;
  onQueueVideo: (video: Video) => Promise<void>;
}

function VideoUrlTab({ activeDownloadStatusByVideoNo, onQueueVideo }: VideoUrlTabProps) {
  const [url, setURL] = useState('');
  const [video, setVideo] = useState<Video>();
  const [loading, setLoading] = useState(false);
  const [resolved, setResolved] = useState(false);
  const [error, setError] = useState('');

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

  return (
    <div>
      <Title level={3} className="!mb-2 !text-slate-100">
        URL 직접 입력
      </Title>
      <Paragraph className="!text-slate-400">
        다운로드할 치지직 VOD URL을 입력하면 영상 정보를 확인한 뒤 바로 Queue에 추가할 수 있습니다.
      </Paragraph>

      <Input.Search
        value={url}
        onChange={(event) => setURL(event.target.value)}
        onSearch={() => void resolve()}
        enterButton="영상 확인"
        placeholder="https://chzzk.naver.com/video/{videoNo}"
        size="large"
        loading={loading}
      />

      {error && <Alert className="mt-4" type="error" showIcon message={error} />}

      {video && (
        <div className="mt-5 max-w-xl">
          <VideoCard
            video={video}
            downloadStatus={activeDownloadStatusByVideoNo.get(video.videoNo)}
            onQueue={onQueueVideo}
          />
        </div>
      )}

      {resolved && !loading && !video && !error && (
        <Empty className="mt-12" description="VOD 정보를 찾을 수 없습니다." />
      )}
    </div>
  );
}

export default VideoUrlTab;
