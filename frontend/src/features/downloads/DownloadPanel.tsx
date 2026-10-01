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
  Statistic,
  Tag,
  Typography,
} from 'antd';
import { useEffect, useMemo, useState } from 'react';

import { cancelDownload, getDownloadToolchainStatus } from '../../lib/backend';
import type { DownloadTask, ToolchainStatus, ToolStatus } from '../../types/download';

const { Paragraph, Text, Title } = Typography;

interface DownloadPanelProps {
  tasks: DownloadTask[];
}

const statusMeta: Record<
  DownloadTask['status'],
  { label: string; color?: string; progressStatus: 'normal' | 'active' | 'success' | 'exception' }
> = {
  queued: { label: '대기 중', progressStatus: 'normal' },
  running: { label: '다운로드 중', color: 'processing', progressStatus: 'active' },
  completed: { label: '완료', color: 'success', progressStatus: 'success' },
  failed: { label: '실패', color: 'error', progressStatus: 'exception' },
  cancelled: { label: '취소됨', progressStatus: 'normal' },
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

  const unavailable = [
    !status.ytDlp.available && status.ytDlp,
    !status.ffmpeg.available && status.ffmpeg,
    !status.ffprobe.available && status.ffprobe,
  ].filter((tool): tool is ToolStatus => Boolean(tool));

  return (
    <Alert
      type="warning"
      showIcon
      message="다운로드 도구를 확인해 주세요"
      description={
        <Space direction="vertical" size={2}>
          <Text className="!text-xs">
            사용할 수 없는 도구: {unavailable.map((tool) => tool.name).join(', ')}
          </Text>
          {unavailable.map((tool) => (
            <Text key={tool.name} className="!text-xs !text-slate-500">
              {tool.name}: {tool.error || '실행 파일을 확인할 수 없습니다.'}
            </Text>
          ))}
        </Space>
      }
    />
  );
}

function DownloadTaskCard({
  task,
  queuePosition,
}: {
  task: DownloadTask;
  queuePosition?: number;
}) {
  const { message } = AntdApp.useApp();
  const [cancelling, setCancelling] = useState(false);
  const meta = statusMeta[task.status];
  const active = task.status === 'queued' || task.status === 'running';
  const total = task.progress.totalBytes > 0
    ? `${task.progress.totalBytesEstimated ? '약 ' : ''}${formatBytes(task.progress.totalBytes)}`
    : '-';

  const handleCancel = async () => {
    if (!active || cancelling) {
      return;
    }

    setCancelling(true);
    try {
      const accepted = await cancelDownload(task.taskId);
      if (!accepted) {
        message.warning('취소할 다운로드를 찾을 수 없습니다.');
      }
    } catch (cause) {
      message.error(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setCancelling(false);
    }
  };

  return (
    <Card className="border-slate-800 bg-slate-900/80" styles={{ body: { padding: 16 } }}>
      <div className="grid gap-4 md:grid-cols-[180px_minmax(0,1fr)]">
        <div className="overflow-hidden rounded-lg bg-slate-950">
          {task.thumbnailImageUrl ? (
            <Image
              src={task.thumbnailImageUrl}
              alt={task.videoTitle}
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
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div className="min-w-0">
              <Space size={8} wrap>
                <Tag color={meta.color}>{meta.label}</Tag>
                {queuePosition !== undefined && <Tag>대기 {queuePosition}번째</Tag>}
                <Text className="!text-xs !text-slate-500">{task.channelName}</Text>
              </Space>
              <Title level={5} ellipsis={{ rows: 2 }} className="!mb-0 !mt-2 !text-slate-100">
                {task.videoTitle}
              </Title>
            </div>

            {active && (
              <Button danger size="small" loading={cancelling} onClick={() => void handleCancel()}>
                {task.status === 'queued' ? '대기 취소' : '다운로드 취소'}
              </Button>
            )}
          </div>

          {task.status === 'queued' ? (
            <div className="mt-4 rounded-lg border border-slate-800 bg-slate-950/50 px-3 py-2">
              <Text className="!text-xs !text-slate-400">
                앞선 작업이 끝나면 자동으로 다운로드를 시작합니다.
              </Text>
            </div>
          ) : (
            <div className="mt-4">
              <Progress
                percent={Math.round(task.progress.percent * 10) / 10}
                status={meta.progressStatus}
              />
              <div className="flex flex-wrap gap-x-4 gap-y-1">
                <Text className="!text-xs !text-slate-500">
                  {formatBytes(task.progress.downloadedBytes)} / {total}
                </Text>
                <Text className="!text-xs !text-slate-500">
                  속도 {formatSpeed(task.progress.speedBytesPerSecond)}
                </Text>
                <Text className="!text-xs !text-slate-500">
                  ETA {formatETA(task.progress.etaSeconds)}
                </Text>
              </div>
            </div>
          )}

          {task.finalPath && (
            <Paragraph className="!mb-0 !mt-3 !text-xs !text-slate-300">
              완료 파일: {task.finalPath}
            </Paragraph>
          )}

          {!task.finalPath && task.outputDir && (
            <Paragraph className="!mb-0 !mt-3 !text-xs !text-slate-500">
              저장 위치: {task.outputDir}
            </Paragraph>
          )}

          {task.error && task.status === 'failed' && (
            <Alert className="mt-3" type="error" showIcon message={task.error} />
          )}

          {task.status === 'cancelled' && (
            <Alert className="mt-3" type="info" showIcon message="다운로드가 취소되었습니다." />
          )}
        </div>
      </div>
    </Card>
  );
}

function DownloadPanel({ tasks }: DownloadPanelProps) {
  const { message } = AntdApp.useApp();
  const [toolchain, setToolchain] = useState<ToolchainStatus>();
  const [preparing, setPreparing] = useState(true);

  useEffect(() => {
    let active = true;
    setPreparing(true);

    getDownloadToolchainStatus()
      .then((status) => {
        if (active) {
          setToolchain(status);
        }
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

  const counts = useMemo(
    () => ({
      total: tasks.length,
      queued: tasks.filter((task) => task.status === 'queued').length,
      running: tasks.filter((task) => task.status === 'running').length,
      completed: tasks.filter((task) => task.status === 'completed').length,
      failed: tasks.filter((task) => task.status === 'failed').length,
      cancelled: tasks.filter((task) => task.status === 'cancelled').length,
    }),
    [tasks],
  );

  const queuePositions = useMemo(() => {
    const positions = new Map<string, number>();
    let position = 0;
    tasks.forEach((task) => {
      if (task.status === 'queued') {
        position += 1;
        positions.set(task.taskId, position);
      }
    });
    return positions;
  }, [tasks]);

  if (preparing) {
    return (
      <Card className="border-slate-800 bg-slate-900/80">
        <Skeleton active paragraph={{ rows: 6 }} />
      </Card>
    );
  }

  return (
    <div className="space-y-6">
      {toolchain && <ToolchainAlert status={toolchain} />}

      <Card className="border-slate-800 bg-slate-900/80">
        <div className="grid grid-cols-2 gap-4 md:grid-cols-3 xl:grid-cols-6">
          <Statistic title="전체" value={counts.total} />
          <Statistic title="대기" value={counts.queued} />
          <Statistic title="진행" value={counts.running} />
          <Statistic title="완료" value={counts.completed} />
          <Statistic title="실패" value={counts.failed} />
          <Statistic title="취소" value={counts.cancelled} />
        </div>
      </Card>

      {tasks.length === 0 ? (
        <Card className="border-slate-800 bg-slate-900/80">
          <Empty description="등록된 다운로드 작업이 없습니다. VOD 목록에서 Queue에 추가해 주세요." />
        </Card>
      ) : (
        <div className="space-y-4">
          {tasks.map((task) => (
            <DownloadTaskCard
              key={task.taskId}
              task={task}
              queuePosition={queuePositions.get(task.taskId)}
            />
          ))}
        </div>
      )}
    </div>
  );
}

export default DownloadPanel;
