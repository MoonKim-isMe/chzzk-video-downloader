import {
  Alert,
  App as AntdApp,
  Button,
  Card,
  Empty,
  Image,
  Progress,
  Skeleton,
  Space,
  Tag,
  Typography,
} from 'antd';
import { useEffect, useMemo, useState } from 'react';

import {
  cancelDownload,
  getDefaultDownloadDir,
  getDownloadToolchainStatus,
  startDownload,
} from '../../lib/backend';
import type { DownloadTask, ToolchainStatus } from '../../types/download';
import type { Video } from '../../types/video';

const { Paragraph, Text, Title } = Typography;

interface DownloadPanelProps {
  selectedVideo?: Video;
  currentTask?: DownloadTask;
  onTaskStarted: (task: DownloadTask) => void;
}

const statusMeta: Record<DownloadTask['status'], { label: string; color?: string }> = {
  queued: { label: '대기 중' },
  running: { label: '다운로드 중', color: 'processing' },
  completed: { label: '완료', color: 'success' },
  failed: { label: '실패', color: 'error' },
  cancelled: { label: '취소됨' },
};

function formatBytes(bytes: number) {
  if (!Number.isFinite(bytes) || bytes <= 0) {
    return '-';
  }

  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let value = bytes;
  let index = 0;
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024;
    index += 1;
  }
  return `${value.toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
}

function formatSpeed(bytesPerSecond: number) {
  const value = formatBytes(bytesPerSecond);
  return value === '-' ? '-' : `${value}/s`;
}

function formatETA(seconds: number) {
  if (!Number.isFinite(seconds) || seconds <= 0) {
    return '-';
  }

  const safe = Math.floor(seconds);
  const hours = Math.floor(safe / 3600);
  const minutes = Math.floor((safe % 3600) / 60);
  const remain = safe % 60;
  if (hours > 0) {
    return `${hours}시간 ${minutes}분 ${remain}초`;
  }
  if (minutes > 0) {
    return `${minutes}분 ${remain}초`;
  }
  return `${remain}초`;
}

function ToolchainAlert({ status }: { status: ToolchainStatus }) {
  if (status.downloadReady && status.mergeReady) {
    return (
      <Alert
        type="success"
        showIcon
        message="다운로드 도구 준비 완료"
        description={`yt-dlp ${status.ytDlp.version || '확인됨'} · ffmpeg/ffprobe 확인됨`}
      />
    );
  }

  const missing = [
    !status.ytDlp.available && 'yt-dlp',
    !status.ffmpeg.available && 'ffmpeg',
    !status.ffprobe.available && 'ffprobe',
  ].filter(Boolean);

  return (
    <Alert
      type="warning"
      showIcon
      message="다운로드 도구를 확인해 주세요"
      description={`사용할 수 없는 도구: ${missing.join(', ')}`}
    />
  );
}

function DownloadPanel({ selectedVideo, currentTask, onTaskStarted }: DownloadPanelProps) {
  const { message } = AntdApp.useApp();
  const [outputDir, setOutputDir] = useState('');
  const [toolchain, setToolchain] = useState<ToolchainStatus>();
  const [preparing, setPreparing] = useState(true);
  const [starting, setStarting] = useState(false);
  const [cancelling, setCancelling] = useState(false);

  useEffect(() => {
    let active = true;
    setPreparing(true);

    Promise.all([getDefaultDownloadDir(), getDownloadToolchainStatus()])
      .then(([directory, status]) => {
        if (!active) {
          return;
        }
        setOutputDir(directory);
        setToolchain(status);
      })
      .catch((cause) => {
        if (active) {
          message.error(cause instanceof Error ? cause.message : String(cause));
        }
      })
      .finally(() => {
        if (active) {
          setPreparing(false);
        }
      });

    return () => {
      active = false;
    };
  }, [message]);

  const busy = currentTask?.status === 'queued' || currentTask?.status === 'running';
  const canStart = Boolean(
    selectedVideo &&
      outputDir &&
      toolchain?.downloadReady &&
      toolchain.mergeReady &&
      !busy &&
      !starting,
  );

  const progressDescription = useMemo(() => {
    if (!currentTask) {
      return '';
    }

    const { progress } = currentTask;
    const total = progress.totalBytes > 0
      ? `${progress.totalBytesEstimated ? '약 ' : ''}${formatBytes(progress.totalBytes)}`
      : '-';

    return `${formatBytes(progress.downloadedBytes)} / ${total} · ${formatSpeed(
      progress.speedBytesPerSecond,
    )} · ETA ${formatETA(progress.etaSeconds)}`;
  }, [currentTask]);

  const progressStatus = currentTask?.status === 'failed'
    ? 'exception'
    : currentTask?.status === 'completed'
      ? 'success'
      : currentTask?.status === 'running'
        ? 'active'
        : 'normal';

  const handleStart = async () => {
    if (!selectedVideo || !canStart) {
      return;
    }

    setStarting(true);
    try {
      const task = await startDownload({
        videoNo: selectedVideo.videoNo,
        videoTitle: selectedVideo.videoTitle,
        channelName: selectedVideo.channel.channelName,
        thumbnailImageUrl: selectedVideo.thumbnailImageUrl,
        url: selectedVideo.videoUrl,
        outputDir,
      });
      onTaskStarted(task);
      message.success('다운로드를 시작했습니다.');
    } catch (cause) {
      message.error(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setStarting(false);
    }
  };

  const handleCancel = async () => {
    if (!currentTask || currentTask.status !== 'running') {
      return;
    }

    setCancelling(true);
    try {
      const accepted = await cancelDownload(currentTask.taskId);
      if (!accepted) {
        message.warning('취소할 다운로드를 찾을 수 없습니다.');
      }
    } catch (cause) {
      message.error(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setCancelling(false);
    }
  };

  if (preparing) {
    return (
      <Card className="border-slate-800 bg-slate-900/80">
        <Skeleton active paragraph={{ rows: 5 }} />
      </Card>
    );
  }

  return (
    <div className="space-y-6">
      {toolchain && <ToolchainAlert status={toolchain} />}

      <Card className="border-slate-800 bg-slate-900/80">
        <Text className="!text-xs !font-semibold !uppercase !tracking-wider !text-slate-500">
          다운로드 대상
        </Text>

        {selectedVideo ? (
          <div className="mt-4 grid gap-4 md:grid-cols-[220px_minmax(0,1fr)]">
            <div className="overflow-hidden rounded-lg bg-slate-950">
              {selectedVideo.thumbnailImageUrl ? (
                <Image
                  src={selectedVideo.thumbnailImageUrl}
                  alt={selectedVideo.videoTitle}
                  preview={false}
                  className="aspect-video !w-full object-cover"
                />
              ) : (
                <div className="flex aspect-video items-center justify-center">
                  <Text className="!text-slate-600">썸네일 없음</Text>
                </div>
              )}
            </div>

            <div className="min-w-0">
              <Text className="!text-xs !text-slate-500">{selectedVideo.channel.channelName}</Text>
              <Title level={4} className="!mb-2 !mt-1 !text-slate-100">
                {selectedVideo.videoTitle}
              </Title>
              <Paragraph className="!mb-3 !text-xs !text-slate-500">
                저장 위치: {outputDir || '확인할 수 없음'}
              </Paragraph>
              <Button type="primary" loading={starting} disabled={!canStart} onClick={handleStart}>
                다운로드 시작
              </Button>
            </div>
          </div>
        ) : (
          <Empty className="mt-4" description="채널 VOD 목록에서 다운로드할 영상을 선택해 주세요." />
        )}
      </Card>

      <Card className="border-slate-800 bg-slate-900/80">
        <Text className="!text-xs !font-semibold !uppercase !tracking-wider !text-slate-500">
          현재 다운로드
        </Text>

        {currentTask ? (
          <div className="mt-4">
            <Space size={8} wrap>
              <Tag color={statusMeta[currentTask.status].color}>{statusMeta[currentTask.status].label}</Tag>
              <Text className="!text-slate-400">{currentTask.channelName}</Text>
            </Space>

            <Title level={4} className="!mb-4 !mt-2 !text-slate-100">
              {currentTask.videoTitle}
            </Title>

            <Progress
              percent={Math.round(currentTask.progress.percent * 10) / 10}
              status={progressStatus}
            />
            <Text className="!text-xs !text-slate-500">{progressDescription}</Text>

            {currentTask.finalPath && (
              <Paragraph className="!mb-0 !mt-4 !text-sm !text-slate-300">
                완료 파일: {currentTask.finalPath}
              </Paragraph>
            )}

            {currentTask.error && currentTask.status !== 'cancelled' && (
              <Alert className="mt-4" type="error" showIcon message={currentTask.error} />
            )}

            {currentTask.status === 'cancelled' && (
              <Alert className="mt-4" type="info" showIcon message="다운로드가 취소되었습니다." />
            )}

            {currentTask.status === 'running' && (
              <Button danger className="mt-4" loading={cancelling} onClick={handleCancel}>
                다운로드 취소
              </Button>
            )}
          </div>
        ) : (
          <Empty className="mt-4" description="진행 중인 다운로드가 없습니다." />
        )}
      </Card>
    </div>
  );
}

export default DownloadPanel;
