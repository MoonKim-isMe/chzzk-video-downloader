import {
  App as AntdApp,
  Button,
  Card,
  Empty,
  Image,
  Progress,
  Skeleton,
  Tag,
  Typography,
} from 'antd';
import { useEffect, useMemo, useState, type ReactNode } from 'react';

import {
  cancelDownload,
  deleteDownloadTask,
  getDownloadToolchainStatus,
  openDownloadFolder,
  openDownloadLog,
  recoverDownload,
} from '../../lib/backend';
import type { DownloadTask, ToolchainStatus } from '../../types/download';

const { Text, Title } = Typography;

interface DownloadPanelProps {
  tasks: DownloadTask[];
  onTaskRemoved: (taskId: string) => void;
  onTaskRecovered: (previousTaskId: string, task: DownloadTask) => void;
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

function DownloadCapability({ status }: { status: ToolchainStatus }) {
  const ready = status.downloadReady && status.mergeReady;

  return (
    <div className={`download-capability ${ready ? 'is-ready' : 'is-warning'}`}>
      <span className="download-capability-icon">{ready ? '✓' : '!'}</span>
      <div className="min-w-0">
        <Text strong className="block">
          {ready ? '영상 다운로드 준비 완료' : '영상 다운로드 기능을 사용할 수 없습니다'}
        </Text>
        <Text className="app-muted block !text-xs">
          {ready
            ? '영상 다운로드와 파일 저장 기능을 사용할 수 있습니다.'
            : '다운로드 실행 환경을 확인한 뒤 앱을 다시 실행해 주세요.'}
        </Text>
      </div>
    </div>
  );
}

function DownloadThumbnail({ task }: { task: DownloadTask }) {
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    setFailed(false);
  }, [task.thumbnailImageUrl]);

  return (
    <div className="download-task-thumbnail">
      {task.thumbnailImageUrl && !failed ? (
        <Image
          src={task.thumbnailImageUrl}
          alt={task.videoTitle}
          preview={false}
          className="!h-full !w-full object-cover"
          onError={() => setFailed(true)}
        />
      ) : (
        <div className="download-task-thumbnail-placeholder">
          <Text className="!text-center !text-xs !text-slate-500">
            썸네일이 없거나 불러오지 못했습니다
          </Text>
        </div>
      )}
    </div>
  );
}

interface DownloadTaskRowProps {
  task: DownloadTask;
  queuePosition?: number;
  onTaskRemoved: (taskId: string) => void;
  onTaskRecovered: (previousTaskId: string, task: DownloadTask) => void;
}

function DownloadTaskRow({
  task,
  queuePosition,
  onTaskRemoved,
  onTaskRecovered,
}: DownloadTaskRowProps) {
  const { message } = AntdApp.useApp();
  const [working, setWorking] = useState(false);
  const meta = statusMeta[task.status];
  const active = task.status === 'queued' || task.status === 'running';
  const fallbackPreparing = task.progress.status === 'fallback_preparing';
  const fallbackDownloading = task.progress.status === 'fallback_downloading';
  const fallbackActive = task.status === 'running' && (fallbackPreparing || fallbackDownloading);
  const total = task.progress.totalBytes > 0
    ? `${task.progress.totalBytesEstimated ? '약 ' : ''}${formatBytes(task.progress.totalBytes)}`
    : '-';

  const handleCancel = async () => {
    if (!active || working) {
      return;
    }

    setWorking(true);
    try {
      const accepted = await cancelDownload(task.taskId);
      if (!accepted) {
        message.warning('이미 종료되었거나 취소할 수 없는 다운로드입니다.');
        return;
      }
      onTaskRemoved(task.taskId);
    } catch (cause) {
      message.error(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setWorking(false);
    }
  };

  const handleRecover = async () => {
    if (task.status !== 'failed' || task.errorCode !== 'partial_data_conflict' || working) {
      return;
    }

    setWorking(true);
    try {
      const recovered = await recoverDownload(task.taskId);
      onTaskRecovered(task.taskId, recovered);
      message.success('임시 파일을 정리하고 다운로드를 다시 시작했습니다.');
    } catch (cause) {
      message.error(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setWorking(false);
    }
  };

  const handleOpenLog = async () => {
    if (!task.logPath || working) {
      return;
    }

    setWorking(true);
    try {
      await openDownloadLog(task.taskId);
    } catch (cause) {
      message.error(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setWorking(false);
    }
  };

  const handleOpenFolder = async () => {
    setWorking(true);
    try {
      await openDownloadFolder(task.taskId);
    } catch (cause) {
      message.error(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setWorking(false);
    }
  };

  const handleDelete = async () => {
    setWorking(true);
    try {
      await deleteDownloadTask(task.taskId);
      onTaskRemoved(task.taskId);
      message.success('다운로드 목록에서 삭제했습니다.');
    } catch (cause) {
      message.error(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setWorking(false);
    }
  };

  return (
    <article className={`download-task-row is-${task.status}`}>
      <DownloadThumbnail task={task} />

      <div className="download-task-main">
        <div className="download-task-heading">
          <div className="min-w-0">
            <div className="download-task-badges">
              <Tag color={meta.color}>{meta.label}</Tag>
              {fallbackActive && <Tag color="warning">대체 방식 재시도</Tag>}
              {queuePosition !== undefined && <Tag>대기 {queuePosition}번째</Tag>}
              <Text className="app-muted !text-xs">{task.channelName}</Text>
            </div>
            <Title level={5} ellipsis={{ rows: 1 }} className="download-task-title !mb-0 !mt-1">
              {task.videoTitle}
            </Title>
          </div>

          <div className="download-task-actions">
            {active && (
              <Button danger size="small" loading={working} onClick={() => void handleCancel()}>
                취소
              </Button>
            )}
            {task.status === 'completed' && (
              <>
                <Button size="small" loading={working} onClick={() => void handleOpenFolder()}>
                  폴더 열기
                </Button>
                <Button danger size="small" disabled={working} onClick={() => void handleDelete()}>
                  목록에서 삭제
                </Button>
              </>
            )}
            {task.status === 'failed' && (
              <>
                {task.logPath && (
                  <Button size="small" disabled={working} onClick={() => void handleOpenLog()}>
                    로그 파일 열기
                  </Button>
                )}
                {task.errorCode === 'partial_data_conflict' && (
                  <Button type="primary" size="small" loading={working} onClick={() => void handleRecover()}>
                    임시 파일 정리 후 재시도
                  </Button>
                )}
                <Button danger size="small" disabled={working} onClick={() => void handleDelete()}>
                  목록에서 삭제
                </Button>
              </>
            )}
          </div>
        </div>

        {task.status === 'queued' ? (
          <div className="download-task-queued">
            앞선 작업이 끝나면 자동으로 다운로드를 시작합니다.
          </div>
        ) : task.status === 'completed' ? (
          <div className="download-task-completed-meta">
            <span>다운로드 완료</span>
            {task.finalPath && <span className="download-task-path">{task.finalPath}</span>}
          </div>
        ) : (
          <>
            {fallbackActive && (
              <div className="download-task-queued">
                {fallbackPreparing
                  ? '대체 다운로드 방식을 준비하고 있습니다.'
                  : task.progress.downloadedBytes > 0
                    ? '대체 방식으로 다운로드 중입니다.'
                    : '대체 다운로드 서버에 연결 중입니다.'}
              </div>
            )}

            <div className="download-task-progress">
              <Progress
                percent={Math.round(task.progress.percent * 10) / 10}
                status={meta.progressStatus}
                showInfo={!fallbackActive || task.progress.percent > 0}
                size="small"
              />
            </div>

            <div className="download-task-metrics">
              {fallbackActive && task.progress.totalBytes <= 0 ? (
                <>
                  <span>
                    다운로드 {task.progress.downloadedBytes > 0
                      ? formatBytes(task.progress.downloadedBytes)
                      : '0 B'}
                  </span>
                  <span>속도 {formatSpeed(task.progress.speedBytesPerSecond)}</span>
                  <span>전체 크기 계산 중</span>
                </>
              ) : (
                <>
                  <span>{formatBytes(task.progress.downloadedBytes)} / {total}</span>
                  <span>속도 {formatSpeed(task.progress.speedBytesPerSecond)}</span>
                  <span>남은 시간 {formatETA(task.progress.etaSeconds)}</span>
                </>
              )}
            </div>

            {task.status === 'failed' && task.error && (
              <div className="download-task-error">{task.error}</div>
            )}
            {task.status === 'failed' && task.logPath && (
              <div className="download-task-path">오류 로그: {task.logPath}</div>
            )}
          </>
        )}
      </div>
    </article>
  );
}

function DownloadSection({
  title,
  count,
  tasks,
  queuePositions,
  onTaskRemoved,
  onTaskRecovered,
  action,
}: {
  title: string;
  count: number;
  tasks: DownloadTask[];
  queuePositions: Map<string, number>;
  onTaskRemoved: (taskId: string) => void;
  onTaskRecovered: (previousTaskId: string, task: DownloadTask) => void;
  action?: ReactNode;
}) {
  if (tasks.length === 0) {
    return null;
  }

  return (
    <section className="download-section">
      <div className="download-section-heading">
        <Text strong>{title}</Text>
        <span className="download-section-count">{count}</span>
        {action && <div className="ml-auto">{action}</div>}
      </div>
      <div className="download-section-list">
        {tasks.map((task) => (
          <DownloadTaskRow
            key={task.taskId}
            task={task}
            queuePosition={queuePositions.get(task.taskId)}
            onTaskRemoved={onTaskRemoved}
            onTaskRecovered={onTaskRecovered}
          />
        ))}
      </div>
    </section>
  );
}

function DownloadPanel({ tasks, onTaskRemoved, onTaskRecovered }: DownloadPanelProps) {
  const { message } = AntdApp.useApp();
  const [toolchain, setToolchain] = useState<ToolchainStatus>();
  const [preparing, setPreparing] = useState(true);
  const [deletingCompleted, setDeletingCompleted] = useState(false);

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

  const visibleTasks = useMemo(
    () => tasks.filter((task) => task.status !== 'cancelled'),
    [tasks],
  );

  const activeTasks = useMemo(
    () => visibleTasks.filter((task) => task.status === 'queued' || task.status === 'running'),
    [visibleTasks],
  );
  const completedTasks = useMemo(
    () => visibleTasks.filter((task) => task.status === 'completed').slice().reverse(),
    [visibleTasks],
  );
  const failedTasks = useMemo(
    () => visibleTasks.filter((task) => task.status === 'failed').slice().reverse(),
    [visibleTasks],
  );

  const queuePositions = useMemo(() => {
    const positions = new Map<string, number>();
    let position = 0;
    activeTasks.forEach((task) => {
      if (task.status === 'queued') {
        position += 1;
        positions.set(task.taskId, position);
      }
    });
    return positions;
  }, [activeTasks]);

  const counts = {
    active: activeTasks.length,
    completed: completedTasks.length,
    failed: failedTasks.length,
  };

  const handleDeleteCompleted = async () => {
    if (completedTasks.length === 0 || deletingCompleted) {
      return;
    }

    setDeletingCompleted(true);
    let removedCount = 0;
    let failedCount = 0;

    try {
      for (const task of completedTasks) {
        try {
          await deleteDownloadTask(task.taskId);
          onTaskRemoved(task.taskId);
          removedCount += 1;
        } catch {
          failedCount += 1;
        }
      }

      if (failedCount === 0) {
        message.success(`완료 항목 ${removedCount}개를 목록에서 삭제했습니다.`);
      } else if (removedCount > 0) {
        message.warning(`완료 항목 ${removedCount}개를 삭제했고 ${failedCount}개는 삭제하지 못했습니다.`);
      } else {
        message.error('완료 항목을 삭제하지 못했습니다.');
      }
    } finally {
      setDeletingCompleted(false);
    }
  };

  return (
    <Card bordered={false} className="app-panel download-workspace-panel">
      <div className="download-workspace-header">
        <div>
          <Text className="app-eyebrow">DOWNLOADS</Text>
          <Title level={4} className="app-section-title !mb-0 !mt-1">
            다운로드
          </Title>
        </div>

        <div className="download-summary">
          <span><strong>{counts.active}</strong> 진행/대기</span>
          <span><strong>{counts.completed}</strong> 완료</span>
          <span><strong>{counts.failed}</strong> 실패</span>
        </div>
      </div>

      <div className="download-capability-wrap">
        {preparing ? (
          <Skeleton active paragraph={{ rows: 1 }} title={false} />
        ) : toolchain ? (
          <DownloadCapability status={toolchain} />
        ) : (
          <div className="download-capability is-warning">
            <span className="download-capability-icon">!</span>
            <div>
              <Text strong className="block">영상 다운로드 상태를 확인할 수 없습니다</Text>
              <Text className="app-muted block !text-xs">앱을 다시 실행해 주세요.</Text>
            </div>
          </div>
        )}
      </div>

      <div className="download-list-scroll">
        {visibleTasks.length === 0 ? (
          <div className="download-empty">
            <Empty description="다운로드 목록이 비어 있습니다." />
          </div>
        ) : (
          <div className="download-sections">
            <DownloadSection
              title="진행 중"
              count={activeTasks.length}
              tasks={activeTasks}
              queuePositions={queuePositions}
              onTaskRemoved={onTaskRemoved}
              onTaskRecovered={onTaskRecovered}
            />
            <DownloadSection
              title="완료"
              count={completedTasks.length}
              tasks={completedTasks}
              queuePositions={queuePositions}
              onTaskRemoved={onTaskRemoved}
              onTaskRecovered={onTaskRecovered}
              action={(
                <Button
                  size="small"
                  loading={deletingCompleted}
                  onClick={() => void handleDeleteCompleted()}
                >
                  완료 목록 삭제
                </Button>
              )}
            />
            <DownloadSection
              title="실패"
              count={failedTasks.length}
              tasks={failedTasks}
              queuePositions={queuePositions}
              onTaskRemoved={onTaskRemoved}
              onTaskRecovered={onTaskRecovered}
            />
          </div>
        )}
      </div>
    </Card>
  );
}

export default DownloadPanel;
