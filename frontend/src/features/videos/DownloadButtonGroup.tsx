import { Button, Dropdown, Space } from 'antd';
import { useState } from 'react';

import type { DownloadTaskStatus, DownloadType } from '../../types/download';

interface DownloadButtonGroupProps {
  videoStatus?: DownloadTaskStatus;
  audioStatus?: DownloadTaskStatus;
  onQueue: (downloadType: DownloadType) => Promise<void>;
  className?: string;
  size?: 'small' | 'middle' | 'large';
}

function DownloadButtonGroup({
  videoStatus,
  audioStatus,
  onQueue,
  className,
  size = 'middle',
}: DownloadButtonGroupProps) {
  const [queueing, setQueueing] = useState<DownloadType | null>(null);
  const videoActive = videoStatus === 'queued' || videoStatus === 'running';
  const audioActive = audioStatus === 'queued' || audioStatus === 'running';

  const handleQueue = async (downloadType: DownloadType) => {
    if (queueing || (downloadType === 'audio' ? audioActive : videoActive)) {
      return;
    }
    setQueueing(downloadType);
    try {
      await onQueue(downloadType);
    } finally {
      setQueueing(null);
    }
  };

  return (
    <Space.Compact block className={`download-format-group ${className ?? ''}`}>
      <Button
        type="primary"
        size={size}
        className="download-format-button"
        disabled={videoActive || queueing !== null}
        loading={queueing === 'video'}
        onClick={() => void handleQueue('video')}
      >
        동영상
      </Button>
      <Button
        size={size}
        className="download-format-button"
        disabled={audioActive || queueing !== null}
        loading={queueing === 'audio'}
        onClick={() => void handleQueue('audio')}
      >
        MP3
      </Button>
      <Dropdown
        trigger={['click']}
        menu={{
          items: [{ key: 'segment', label: '구간 다운로드 (추후 지원)', disabled: true }],
        }}
      >
        <Button size={size} className="download-format-more" aria-label="구간 다운로드 옵션">
          <span aria-hidden="true">▼</span>
        </Button>
      </Dropdown>
    </Space.Compact>
  );
}

export default DownloadButtonGroup;
