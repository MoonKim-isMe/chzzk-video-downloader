import {
  Alert,
  App as AntdApp,
  Button,
  Card,
  Descriptions,
  Empty,
  Progress,
  Segmented,
  Space,
  Spin,
  Tag,
  Tooltip,
  Typography,
} from 'antd';
import { useEffect, useMemo, useRef, useState } from 'react';

import {
  cancelVideoInspection,
  cancelVideoRepair,
  createVideoCompatibilityPlan,
  createVideoRepairPlan,
  revealVideoRepairFile,
  selectVideoRepairFile,
  startDeepVideoInspection,
  startQuickVideoInspection,
  startVideoRepair,
} from '../../lib/backend';
import { onVideoInspectionProgress, onVideoRepairProgress } from '../../lib/runtime';
import {
  canRepairInspectionResult,
  createInitialVideoRepairSession,
  isVideoRepairBusy,
  preferredVideoInspectionResult,
  type VideoInspectionMode,
  type VideoInspectionProgress,
  type VideoRepairFileInfo,
  type VideoRepairPlan,
  type VideoRepairProgress,
  type VideoRepairSession,
  type VideoRepairStrategy,
} from '../../types/videoRepair';
import VideoInspectionResultView from './VideoInspectionResultView';

const { Text, Title } = Typography;

const supportedFormats = 'MP4 · M4V · MOV · MKV · WebM · AVI · TS · M2TS · MTS';

const repairStrategyLabels: Record<VideoRepairStrategy, string> = {
  none: '복구 불필요',
  compatibility_remux: '편집 호환성 복구',
  remux: 'Remux',
  timestamp_remux: 'Timestamp 정규화 + Remux',
  partial: '손상 데이터 제외 후 복구',
  truncate: '정상 구간까지 부분 복구',
  reencode: '재인코딩',
  unavailable: '복구 방법 없음',
};

const inspectionModeDescriptions: Record<VideoInspectionMode, string> = {
  quick: '컨테이너와 스트림 구조, 대표 구간을 중심으로 빠르게 확인합니다.',
  deep: '영상과 오디오 전체를 디코딩해 손상 위치까지 확인하는 방식입니다.',
};

function formatFileSize(bytes: number) {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const index = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  const value = bytes / (1024 ** index);
  return `${value.toLocaleString('ko-KR', { maximumFractionDigits: index >= 3 ? 2 : 1 })} ${units[index]}`;
}

function formatDuration(seconds?: number) {
  if (seconds === undefined || !Number.isFinite(seconds) || seconds <= 0) return '확인 불가';
  const rounded = Math.round(seconds);
  const hours = Math.floor(rounded / 3600);
  const minutes = Math.floor((rounded % 3600) / 60);
  const remaining = rounded % 60;
  return [hours, minutes, remaining].map((value) => String(value).padStart(2, '0')).join(':');
}

function formatElapsed(seconds?: number) {
  if (seconds === undefined || !Number.isFinite(seconds) || seconds < 0) return '00:00:00';
  const rounded = Math.floor(seconds);
  const hours = Math.floor(rounded / 3600);
  const minutes = Math.floor((rounded % 3600) / 60);
  const remaining = rounded % 60;
  return [hours, minutes, remaining].map((value) => String(value).padStart(2, '0')).join(':');
}

function fileChanged(current: VideoRepairFileInfo | undefined, next: VideoRepairFileInfo) {
  return (
    !current ||
    current.path !== next.path ||
    current.sizeBytes !== next.sizeBytes ||
    current.modifiedUnixMilli !== next.modifiedUnixMilli
  );
}

function RepairProgressView({
  progress,
  plan,
}: {
  progress: VideoRepairProgress;
  plan?: VideoRepairPlan;
}) {
  const percent = Math.max(0, Math.min(100, Math.round(progress.percent)));
  const targetDuration = plan?.endSeconds || plan?.durationSeconds;

  return (
    <div className="video-repair-inspection-progress video-repair-repair-progress">
      <div className="video-repair-progress-summary">
        <div className="video-repair-progress-primary">
          <Text strong className="video-repair-progress-percent">{percent}%</Text>
          <Text className="app-muted video-repair-progress-time">
            ({formatElapsed(progress.processedSeconds)} / {targetDuration && targetDuration > 0
              ? formatDuration(targetDuration)
              : '확인 불가'})
          </Text>
        </div>
        <Text className="app-muted video-repair-progress-stage">{progress.stage}</Text>
      </div>
      <Progress percent={percent} size="small" status="active" showInfo={false} />
    </div>
  );
}

function VideoRepairPanel() {
  const { message } = AntdApp.useApp();
  const [session, setSession] = useState<VideoRepairSession>(createInitialVideoRepairSession);
  const [selectingFile, setSelectingFile] = useState(false);
  const [inspectionProgress, setInspectionProgress] = useState<VideoInspectionProgress>();
  const [repairProgress, setRepairProgress] = useState<VideoRepairProgress>();
  const [repairPlanError, setRepairPlanError] = useState('');
  const [compatibilityPlan, setCompatibilityPlan] = useState<VideoRepairPlan>();
  const [compatibilityPlanError, setCompatibilityPlanError] = useState('');
  const [repairError, setRepairError] = useState('');
  const inspectionCancelRequested = useRef(false);
  const repairCancelRequested = useRef(false);

  const busy = isVideoRepairBusy(session.status);
  const fileInfo = session.file;
  const repairBasisResult = preferredVideoInspectionResult(session);
  const quickResult = session.inspectionResults.quick;
  const deepResult = session.inspectionResults.deep;
  const canRepair = Boolean(session.repairPlan?.executable) && !busy;
  const compatibilityEligible = Boolean(
    repairBasisResult
      && ['normal', 'warning'].includes(repairBasisResult.status)
      && repairBasisResult.repairability === 'not_needed'
      && ['.mp4', '.m4v', '.mov'].includes(repairBasisResult.file.extension.toLowerCase())
      && session.repairResult?.plan.strategy !== 'compatibility_remux',
  );
  const compatibilityRecommended = Boolean(
    repairBasisResult?.status === 'warning'
      && repairBasisResult.repairability === 'not_needed',
  );
  const compatibilityRepairPlan = session.repairPlan?.strategy === 'compatibility_remux'
    ? session.repairPlan
    : undefined;
  const compatibilityRepairResult = session.repairResult?.plan.strategy === 'compatibility_remux'
    ? session.repairResult
    : undefined;
  const compatibilityRepairProgress = repairProgress?.strategy === 'compatibility_remux'
    ? repairProgress
    : undefined;
  const compatibilityWorkflowActive = compatibilityEligible
    || Boolean(compatibilityRepairPlan || compatibilityRepairResult || compatibilityRepairProgress);
  const compatibilityDisplayPlan = compatibilityRepairPlan ?? compatibilityPlan ?? compatibilityRepairResult?.plan;
  const generalRepairVisible = Boolean(
    (session.repairPlan && session.repairPlan.strategy !== 'compatibility_remux')
      || (session.repairResult && session.repairResult.plan.strategy !== 'compatibility_remux')
      || (session.status === 'repairing' && repairProgress?.strategy !== 'compatibility_remux')
      || repairPlanError
      || (repairError && !compatibilityWorkflowActive),
  );

  const repairDisabledReason = !repairBasisResult
    ? '검사 결과가 있어야 복구할 수 있습니다.'
    : repairBasisResult.repairability === 'not_needed'
      ? '이 파일은 복구가 필요하지 않습니다.'
      : repairBasisResult.repairability === 'impossible' || repairBasisResult.status === 'failed'
        ? '현재 검사 결과로는 복구할 수 없습니다.'
        : !session.repairPlan
          ? repairPlanError || '복구 계획을 준비하고 있습니다.'
          : !session.repairPlan.executable
            ? session.repairPlan.summary
            : busy
              ? '검사 또는 복구 작업이 진행 중입니다.'
              : undefined;

  useEffect(() => {
    return onVideoInspectionProgress((progress) => {
      setInspectionProgress((current) => {
        if (session.file?.path && progress.path !== session.file.path) return current;
        return progress;
      });
    });
  }, [session.file?.path]);

  useEffect(() => {
    return onVideoRepairProgress((progress) => {
      setRepairProgress((current) => {
        if (session.repairPlan?.sourcePath && progress.sourcePath !== session.repairPlan.sourcePath) {
          return current;
        }
        return progress;
      });
    });
  }, [session.repairPlan?.sourcePath]);

  useEffect(() => {
    const result = repairBasisResult;
    setRepairPlanError('');

    if (!result || !canRepairInspectionResult(result)) {
      setSession((current) => current.repairPlan ? { ...current, repairPlan: undefined } : current);
      return;
    }

    let active = true;
    createVideoRepairPlan(result)
      .then((plan) => {
        if (!active) return;
        setSession((current) => {
          const currentBasis = preferredVideoInspectionResult(current);
          return currentBasis === result ? { ...current, repairPlan: plan } : current;
        });
      })
      .catch((cause) => {
        if (!active) return;
        setRepairPlanError(cause instanceof Error ? cause.message : String(cause));
        setSession((current) => {
          const currentBasis = preferredVideoInspectionResult(current);
          return currentBasis === result ? { ...current, repairPlan: undefined } : current;
        });
      });

    return () => {
      active = false;
    };
  }, [repairBasisResult]);

  useEffect(() => {
    const result = repairBasisResult;
    setCompatibilityPlanError('');

    if (!result || !compatibilityEligible) {
      setCompatibilityPlan(undefined);
      return;
    }

    let active = true;
    createVideoCompatibilityPlan(result)
      .then((plan) => {
        if (active) setCompatibilityPlan(plan);
      })
      .catch((cause) => {
        if (!active) return;
        setCompatibilityPlan(undefined);
        setCompatibilityPlanError(cause instanceof Error ? cause.message : String(cause));
      });

    return () => {
      active = false;
    };
  }, [repairBasisResult, compatibilityEligible]);

  const metadataSource = useMemo(() => {
    if (!fileInfo) return null;
    return fileInfo.containerSource === 'probe' ? '파일 분석' : '확장자 기준';
  }, [fileInfo]);

  const handleSelectFile = async () => {
    if (busy || selectingFile) return;
    setSelectingFile(true);
    setRepairError('');
    setRepairPlanError('');
    setCompatibilityPlanError('');
    setCompatibilityPlan(undefined);
    try {
      const selected = await selectVideoRepairFile(fileInfo?.path ?? '');
      if (!selected.path) return;

      setSession((current) => {
        if (!fileChanged(current.file, selected)) {
          return {
            ...current,
            file: selected,
            status: current.hasInspectionResult ? current.status : 'ready',
            error: undefined,
          };
        }
        return {
          ...createInitialVideoRepairSession(),
          file: selected,
          inspectionMode: current.inspectionMode,
          status: 'ready',
        };
      });
      setInspectionProgress(undefined);
      setRepairProgress(undefined);
    } catch (cause) {
      message.error(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setSelectingFile(false);
    }
  };

  const handleInspectionModeChange = (value: string | number) => {
    setSession((current) => ({
      ...current,
      inspectionMode: value as VideoInspectionMode,
    }));
  };

  const runInspection = async (mode: VideoInspectionMode, targetFile: VideoRepairFileInfo) => {
    if (busy) return;

    inspectionCancelRequested.current = false;
    setRepairError('');
    setRepairPlanError('');
    setInspectionProgress({
      mode,
      path: targetFile.path,
      percent: 0,
      stage: mode === 'quick' ? '빠른 검사 준비 중' : '정밀 검사 준비 중',
    });
    setSession((current) => {
      const previousResult = preferredVideoInspectionResult(current);
      return {
        ...current,
        file: targetFile,
        inspectionMode: mode,
        status: 'inspecting',
        result: previousResult,
        repairPlan: previousResult ? current.repairPlan : undefined,
        hasInspectionResult: Boolean(previousResult),
        error: undefined,
      };
    });

    try {
      const result = mode === 'quick'
        ? await startQuickVideoInspection(targetFile.path)
        : await startDeepVideoInspection(targetFile.path);
      setSession((current) => {
        const inspectionResults = {
          ...current.inspectionResults,
          [mode]: result,
        };
        const preferredResult = inspectionResults.deep ?? inspectionResults.quick ?? result;
        return {
          ...current,
          file: result.file,
          status: result.status === 'failed' ? 'failed' : 'inspected',
          result: preferredResult,
          inspectionResults,
          hasInspectionResult: true,
          completedInspectionModes: current.completedInspectionModes.includes(mode)
            ? current.completedInspectionModes
            : [...current.completedInspectionModes, mode],
          error: undefined,
        };
      });
      setInspectionProgress({
        mode,
        path: targetFile.path,
        percent: 100,
        stage: mode === 'quick' ? '빠른 검사 완료' : '정밀 검사 완료',
        processedSeconds: result.file.durationSeconds,
      });
    } catch (cause) {
      const errorMessage = cause instanceof Error ? cause.message : String(cause);
      const cancelled = inspectionCancelRequested.current || errorMessage.includes('취소');
      setSession((current) => {
        const previousResult = preferredVideoInspectionResult(current);
        return {
          ...current,
          status: previousResult ? 'inspected' : cancelled ? 'ready' : 'failed',
          result: previousResult,
          repairPlan: previousResult ? current.repairPlan : undefined,
          hasInspectionResult: Boolean(previousResult),
          error: previousResult || cancelled ? undefined : errorMessage,
        };
      });
      setInspectionProgress(undefined);
      if (!cancelled) message.error(errorMessage);
    } finally {
      inspectionCancelRequested.current = false;
    }
  };

  const handleStartInspection = async () => {
    if (!fileInfo) return;
    await runInspection(session.inspectionMode, fileInfo);
  };

  const handleCancelInspection = async () => {
    inspectionCancelRequested.current = true;
    try {
      await cancelVideoInspection();
    } catch (cause) {
      inspectionCancelRequested.current = false;
      message.error(cause instanceof Error ? cause.message : String(cause));
    }
  };

  const handleStartRepair = async (requestedPlan?: VideoRepairPlan) => {
    const plan = requestedPlan ?? session.repairPlan;
    if (!plan?.executable || busy) return;

    repairCancelRequested.current = false;
    setRepairError('');
    setRepairProgress({
      sourcePath: plan.sourcePath,
      outputPath: plan.outputPath,
      strategy: plan.strategy,
      percent: 0,
      stage: '복구 준비 중',
    });
    setSession((current) => ({
      ...current,
      repairPlan: plan,
      status: 'repairing',
      error: undefined,
    }));

    try {
      const repairResult = await startVideoRepair(plan);
      const autoInspection = repairResult.autoInspection;
      setSession((current) => ({
        ...current,
        file: repairResult.outputFile,
        inspectionMode: 'quick',
        status: 'repaired',
        result: autoInspection,
        inspectionResults: autoInspection ? { quick: autoInspection } : {},
        hasInspectionResult: Boolean(autoInspection),
        completedInspectionModes: autoInspection ? ['quick'] : [],
        repairPlan: undefined,
        repairResult,
        error: undefined,
      }));
      setInspectionProgress(autoInspection ? {
        mode: 'quick',
        path: repairResult.outputFile.path,
        percent: 100,
        stage: '복구 결과 빠른 검사 완료',
        processedSeconds: repairResult.outputFile.durationSeconds,
      } : undefined);
      setRepairProgress({
        sourcePath: plan.sourcePath,
        outputPath: repairResult.outputFile.path,
        strategy: plan.strategy,
        percent: 100,
        stage: '복구 완료',
        processedSeconds: repairResult.outputFile.durationSeconds,
        elapsedSeconds: repairResult.elapsedSeconds,
      });
      message.success(plan.strategy === 'compatibility_remux'
        ? '편집 호환성 복구가 완료되었습니다.'
        : '동영상 복구가 완료되었습니다.');
    } catch (cause) {
      const errorMessage = cause instanceof Error ? cause.message : String(cause);
      const cancelled = repairCancelRequested.current || errorMessage.includes('취소');
      setSession((current) => ({
        ...current,
        status: current.hasInspectionResult ? 'inspected' : 'ready',
      }));
      setRepairProgress(undefined);
      setRepairError(cancelled ? '' : errorMessage);
      if (!cancelled) message.error(errorMessage);
    } finally {
      repairCancelRequested.current = false;
    }
  };

  const handleCancelRepair = async () => {
    repairCancelRequested.current = true;
    try {
      await cancelVideoRepair();
    } catch (cause) {
      repairCancelRequested.current = false;
      message.error(cause instanceof Error ? cause.message : String(cause));
    }
  };

  const handleDeepInspectRepairResult = async () => {
    const repairedFile = session.repairResult?.outputFile;
    if (!repairedFile || busy) return;
    await runInspection('deep', repairedFile);
  };

  const handleRecommendedDeepInspection = async () => {
    if (!fileInfo || busy) return;
    await runInspection('deep', fileInfo);
  };

  const handleRevealFile = async (path: string) => {
    try {
      await revealVideoRepairFile(path);
    } catch (cause) {
      message.error(cause instanceof Error ? cause.message : String(cause));
    }
  };

  const workflowStatusLabel = session.status === 'inspecting'
    ? '검사 중'
    : session.status === 'repairing'
      ? '복구 중'
      : session.status === 'repaired'
        ? '복구 완료'
        : session.status === 'inspected'
          ? '검사 완료'
          : session.status === 'failed'
            ? '검사 실패'
            : '검사 준비';

  return (
    <Card bordered={false} className="app-panel video-repair-workspace-panel">
      <div className="video-repair-header">
        <div>
          <Text className="app-eyebrow">VIDEO REPAIR</Text>
          <Title level={4} className="app-section-title !mb-0 !mt-1">동영상 검사 및 복구</Title>
          <Text className="app-muted">
            로컬 동영상 파일을 선택한 뒤 검사 결과에 따라 안전한 복구 방법을 실행할 수 있습니다.
          </Text>
        </div>
        <Button type="primary" loading={selectingFile} disabled={busy} onClick={handleSelectFile}>
          {fileInfo ? '다른 동영상 선택' : '동영상 선택'}
        </Button>
      </div>

      <div className="video-repair-scroll">
        {selectingFile ? (
          <div className="video-repair-file-switching">
            <Spin size="large" />
            <div className="video-repair-file-switching-copy">
              <Text strong className="video-repair-file-switching-title">
                {fileInfo ? '동영상 변경 중입니다' : '동영상 불러오는 중입니다'}
              </Text>
              <Text className="app-muted">
                파일 선택과 동영상 정보 확인이 끝나면 화면이 자동으로 갱신됩니다.
              </Text>
            </div>
          </div>
        ) : (
          <>
            <div className="video-repair-layout">
          <Card bordered={false} className="video-repair-section">
            <div className="video-repair-section-heading">
              <div>
                <Text strong>선택한 동영상</Text>
                <div className="video-repair-format-list">지원 형식: {supportedFormats}</div>
              </div>
              {fileInfo && <Tag>{fileInfo.extension.toUpperCase()}</Tag>}
            </div>

            {!fileInfo ? (
              <div className="video-repair-file-empty">
                <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="검사할 동영상 파일을 선택해 주세요.">
                  <Button type="primary" loading={selectingFile} onClick={handleSelectFile}>동영상 선택</Button>
                </Empty>
              </div>
            ) : (
              <div className="video-repair-file-summary">
                <div>
                  <Text strong className="video-repair-file-name">{fileInfo.name}</Text>
                  <Text className="video-repair-path" title={fileInfo.path}>{fileInfo.path}</Text>
                </div>
                <Descriptions size="small" column={2}>
                  <Descriptions.Item label="파일 크기">{formatFileSize(fileInfo.sizeBytes)}</Descriptions.Item>
                  <Descriptions.Item label="재생 시간">{formatDuration(fileInfo.durationSeconds)}</Descriptions.Item>
                  <Descriptions.Item label="컨테이너">
                    <Space size={4}>
                      <span>{fileInfo.container}</span>
                      <Tag className="video-repair-metadata-source">{metadataSource}</Tag>
                    </Space>
                  </Descriptions.Item>
                  <Descriptions.Item label="상태">
                    <Tag color={session.status === 'failed' ? 'error' : session.status === 'repaired' || session.status === 'inspected' ? 'success' : 'processing'}>
                      {workflowStatusLabel}
                    </Tag>
                  </Descriptions.Item>
                </Descriptions>
                {!fileInfo.metadataAvailable && fileInfo.metadataError && (
                  <Alert
                    showIcon
                    type="warning"
                    message="영상 기본 정보를 일부 확인하지 못했습니다."
                    description="파일 선택은 유지됩니다. 정밀 검사에서 손상 여부와 추가 정보를 확인할 수 있습니다."
                  />
                )}
              </div>
            )}
          </Card>

          <Card bordered={false} className="video-repair-section">
            <div className="video-repair-section-heading">
              <div>
                <Text strong>검사 방식</Text>
                <div className="video-repair-format-list">파일을 선택한 뒤 원하는 검사 방식을 선택해 주세요.</div>
              </div>
              <Tag>{session.inspectionMode === 'quick' ? '빠른 검사' : '정밀 검사'}</Tag>
            </div>

            <div className="video-repair-mode-block">
              <Segmented
                block
                className="video-repair-mode-segmented"
                value={session.inspectionMode}
                disabled={busy}
                options={[
                  { label: '빠른 검사', value: 'quick' },
                  { label: '정밀 검사', value: 'deep' },
                ]}
                onChange={handleInspectionModeChange}
              />
              <div className="video-repair-mode-description">
                {inspectionModeDescriptions[session.inspectionMode]}
              </div>

              {session.status === 'inspecting' && inspectionProgress && (
                <div className="video-repair-inspection-progress">
                  <div className="video-repair-inspection-progress-heading">
                    <Text>{inspectionProgress.stage}</Text>
                    <Text className="app-muted">
                      {inspectionProgress.mode === 'quick' && inspectionProgress.sampleCount && inspectionProgress.sampleIndex
                        ? `${inspectionProgress.sampleIndex}/${inspectionProgress.sampleCount}`
                        : inspectionProgress.mode === 'deep'
                          ? [
                              inspectionProgress.processedSeconds !== undefined && fileInfo?.durationSeconds
                                ? `${formatDuration(inspectionProgress.processedSeconds)} / ${formatDuration(fileInfo.durationSeconds)}`
                                : undefined,
                              inspectionProgress.speed ? `${inspectionProgress.speed.toFixed(2)}x` : undefined,
                              `경과 ${formatElapsed(inspectionProgress.elapsedSeconds)}`,
                            ].filter(Boolean).join(' · ')
                          : ''}
                    </Text>
                  </div>
                  <Progress
                    percent={Math.max(0, Math.min(100, Math.round(inspectionProgress.percent)))}
                    size="small"
                    status="active"
                  />
                </div>
              )}

              {session.error && (
                <Alert
                  showIcon
                  type="error"
                  message={session.inspectionMode === 'quick' ? '빠른 검사 실패' : '정밀 검사 실패'}
                  description={session.error}
                />
              )}

              {quickResult?.deepInspectionRecommended && !deepResult && session.status !== 'inspecting' && (
                <Alert
                  showIcon
                  type="warning"
                  message="정밀 검사를 권장합니다"
                  description={quickResult.deepInspectionReason || '빠른 검사만으로 전체 구간 상태를 확정하기 어렵습니다.'}
                  action={
                    <Button size="small" disabled={busy || !fileInfo} onClick={handleRecommendedDeepInspection}>
                      정밀 검사 시작
                    </Button>
                  }
                />
              )}

              <div className="video-repair-actions">
                {session.status === 'inspecting' ? (
                  <Button danger onClick={handleCancelInspection}>검사 취소</Button>
                ) : (
                  <Tooltip title={!fileInfo ? '동영상 파일을 먼저 선택해 주세요.' : undefined}>
                    <span>
                      <Button type="primary" disabled={!fileInfo || busy} onClick={handleStartInspection}>
                        검사 시작
                      </Button>
                    </span>
                  </Tooltip>
                )}

                {session.status === 'repairing' ? (
                  <Button danger onClick={handleCancelRepair}>복구 취소</Button>
                ) : (
                  <Tooltip title={repairDisabledReason}>
                    <span>
                      <Button disabled={!canRepair} onClick={() => void handleStartRepair()}>복구</Button>
                    </span>
                  </Tooltip>
                )}
              </div>
            </div>
          </Card>
        </div>

        {compatibilityWorkflowActive && (
          <Card bordered={false} className="video-repair-section video-repair-compatibility-card !mt-3.5">
            <div className="video-repair-section-heading">
              <div>
                <Text strong>편집 프로그램에서 열리지 않는다면 호환성 복구를 진행해주세요</Text>
                <div className="video-repair-format-list">
                  {compatibilityRecommended
                    ? '컨테이너 또는 스트림 경고가 있지만 일반 손상 복구는 필요하지 않습니다. 호환성 복구를 우선 시도할 수 있습니다.'
                    : '파일 자체가 정상이어도 일부 편집 프로그램에서는 MP4 구조나 타임스탬프 때문에 열리지 않을 수 있습니다.'}
                </div>
              </div>
              <Tag color="orange">
                {compatibilityRepairResult
                  ? '복구 완료'
                  : compatibilityRepairProgress
                    ? '복구 중'
                    : compatibilityRecommended
                      ? '복구 권장'
                      : '선택 기능'}
              </Tag>
            </div>

            <Alert
              showIcon
              type="warning"
              className="video-repair-compatibility-alert"
              message="영상과 오디오는 재인코딩하지 않고 MP4 구조·타임스탬프·인덱스와 출력 파일명만 호환성에 맞게 다시 구성합니다."
            />

            {compatibilityPlanError && (
              <Alert
                showIcon
                type="error"
                message="호환성 복구 계획 생성 실패"
                description={compatibilityPlanError}
              />
            )}

            {repairError && compatibilityWorkflowActive && (
              <Alert showIcon type="error" message="호환성 복구 실패" description={repairError} />
            )}

            {compatibilityDisplayPlan && !compatibilityRepairResult && (
              <div className="video-repair-repair-plan">
                <Descriptions size="small" column={2}>
                  <Descriptions.Item label="처리 방식">무손실 MP4 재구성</Descriptions.Item>
                  <Descriptions.Item label="화질·음질">
                    <Tag color="success">재인코딩 없음</Tag>
                  </Descriptions.Item>
                  <Descriptions.Item label="예상 처리">{compatibilityDisplayPlan.expectedTime}</Descriptions.Item>
                  <Descriptions.Item label="필요 여유 공간">
                    {formatFileSize(compatibilityDisplayPlan.requiredFreeBytes)}
                  </Descriptions.Item>
                  <Descriptions.Item label="출력 파일" span={2}>
                    <Text className="video-repair-path" title={compatibilityDisplayPlan.outputPath}>
                      {compatibilityDisplayPlan.outputPath}
                    </Text>
                  </Descriptions.Item>
                </Descriptions>

                {session.status === 'repairing' && compatibilityRepairProgress ? (
                  <RepairProgressView
                    progress={compatibilityRepairProgress}
                    plan={compatibilityRepairPlan ?? compatibilityDisplayPlan}
                  />
                ) : (
                  <div className="video-repair-actions">
                    <Button
                      type="primary"
                      disabled={busy || !compatibilityDisplayPlan.executable}
                      onClick={() => void handleStartRepair(compatibilityDisplayPlan)}
                    >
                      편집 호환성 복구
                    </Button>
                  </div>
                )}
              </div>
            )}

            {compatibilityRepairResult && (
              <div className="video-repair-repair-result">
                <Alert
                  showIcon
                  type={compatibilityRepairResult.autoInspection?.status === 'normal' ? 'success' : 'warning'}
                  message="호환성 복구 완료"
                  description={
                    compatibilityRepairResult.autoInspection
                      ? `자동 빠른 검사: ${compatibilityRepairResult.autoInspection.summary}`
                      : compatibilityRepairResult.autoInspectionError || '복구 파일의 자동 빠른 검사 결과를 확인하지 못했습니다.'
                  }
                />
                <Descriptions size="small" column={2}>
                  <Descriptions.Item label="결과 파일">
                    {compatibilityRepairResult.outputFile.name}
                  </Descriptions.Item>
                  <Descriptions.Item label="처리 시간">
                    {formatElapsed(compatibilityRepairResult.elapsedSeconds)}
                  </Descriptions.Item>
                  <Descriptions.Item label="복구 방식">
                    {repairStrategyLabels[compatibilityRepairResult.plan.strategy]}
                  </Descriptions.Item>
                  <Descriptions.Item label="자동 빠른 검사">
                    {compatibilityRepairResult.autoInspection ? '완료' : '확인 필요'}
                  </Descriptions.Item>
                  <Descriptions.Item label="저장 경로" span={2}>
                    <Text className="video-repair-path" title={compatibilityRepairResult.outputFile.path}>
                      {compatibilityRepairResult.outputFile.path}
                    </Text>
                  </Descriptions.Item>
                </Descriptions>
                <Space wrap>
                  <Button disabled={busy} onClick={handleDeepInspectRepairResult}>
                    복구 결과 정밀 검사
                  </Button>
                  <Button onClick={() => void handleRevealFile(compatibilityRepairResult.outputFile.path)}>
                    결과 파일 위치 열기
                  </Button>
                </Space>
              </div>
            )}
          </Card>
        )}

        {generalRepairVisible && (
          <Card bordered={false} className="video-repair-section !mt-3.5">
            <div className="video-repair-section-heading">
              <div>
                <Text strong>복구 계획 및 결과</Text>
                <div className="video-repair-format-list">
                  원본 파일은 변경하지 않고 별도의 복구 파일을 생성합니다.
                </div>
              </div>
              {session.repairPlan && <Tag>{repairStrategyLabels[session.repairPlan.strategy]}</Tag>}
            </div>

            {repairPlanError && (
              <Alert showIcon type="error" message="복구 계획 생성 실패" description={repairPlanError} />
            )}
            {repairError && (
              <Alert showIcon type="error" message="복구 실패" description={repairError} />
            )}

            {session.repairPlan && (
              <div className="video-repair-repair-plan">
                <Descriptions size="small" column={2}>
                  <Descriptions.Item label="복구 방식">
                    {repairStrategyLabels[session.repairPlan.strategy]}
                  </Descriptions.Item>
                  <Descriptions.Item label="기준 검사">
                    {session.repairPlan.inspectionMode === 'deep' ? '정밀 검사' : '빠른 검사'}
                  </Descriptions.Item>
                  <Descriptions.Item label="예상 처리">
                    {session.repairPlan.expectedTime}
                  </Descriptions.Item>
                  <Descriptions.Item label="필요 여유 공간">
                    {formatFileSize(session.repairPlan.requiredFreeBytes)}
                  </Descriptions.Item>
                  <Descriptions.Item label="화질">
                    <Tag color={session.repairPlan.qualityLoss ? 'warning' : 'success'}>
                      {session.repairPlan.qualityLoss ? '재인코딩으로 손실 가능' : '재인코딩 없음'}
                    </Tag>
                  </Descriptions.Item>
                  <Descriptions.Item label="구간">
                    <Tag color={session.repairPlan.segmentLoss ? 'warning' : 'success'}>
                      {session.repairPlan.segmentLoss ? '일부 구간 손실 가능' : '구간 손실 없음'}
                    </Tag>
                  </Descriptions.Item>
                  {session.repairPlan.endSeconds !== undefined && session.repairPlan.endSeconds > 0 && (
                    <Descriptions.Item label="복구 종료 위치">
                      {formatDuration(session.repairPlan.endSeconds)}
                    </Descriptions.Item>
                  )}
                  <Descriptions.Item label="출력 파일" span={2}>
                    <Text className="video-repair-path" title={session.repairPlan.outputPath}>
                      {session.repairPlan.outputPath}
                    </Text>
                  </Descriptions.Item>
                </Descriptions>
                <Alert showIcon type="info" message={session.repairPlan.summary} />
              </div>
            )}

            {session.status === 'repairing' && repairProgress && (
              <RepairProgressView progress={repairProgress} plan={session.repairPlan} />
            )}

            {session.repairResult && (
              <div className="video-repair-repair-result">
                <Alert
                  showIcon
                  type={session.repairResult.autoInspection?.status === 'normal' ? 'success' : 'warning'}
                  message="복구 파일 생성 완료"
                  description={
                    session.repairResult.autoInspection
                      ? `자동 빠른 검사: ${session.repairResult.autoInspection.summary}`
                      : session.repairResult.autoInspectionError || '복구 파일의 자동 빠른 검사 결과를 확인하지 못했습니다.'
                  }
                />
                <Descriptions size="small" column={2}>
                  <Descriptions.Item label="결과 파일">
                    {session.repairResult.outputFile.name}
                  </Descriptions.Item>
                  <Descriptions.Item label="처리 시간">
                    {formatElapsed(session.repairResult.elapsedSeconds)}
                  </Descriptions.Item>
                  <Descriptions.Item label="복구 방식">
                    {repairStrategyLabels[session.repairResult.plan.strategy]}
                  </Descriptions.Item>
                  <Descriptions.Item label="자동 빠른 검사">
                    {session.repairResult.autoInspection ? '완료' : '확인 필요'}
                  </Descriptions.Item>
                  <Descriptions.Item label="저장 경로" span={2}>
                    <Text className="video-repair-path" title={session.repairResult.outputFile.path}>
                      {session.repairResult.outputFile.path}
                    </Text>
                  </Descriptions.Item>
                </Descriptions>
                <Space wrap>
                  <Button disabled={busy} onClick={handleDeepInspectRepairResult}>
                    복구 결과 정밀 검사
                  </Button>
                  <Button onClick={() => void handleRevealFile(session.repairResult!.outputFile.path)}>
                    결과 파일 위치 열기
                  </Button>
                </Space>
              </div>
            )}
          </Card>
        )}

            <Card bordered={false} className="video-repair-section !mt-3.5">
          <div className="video-repair-section-heading">
            <div>
              <Text strong>검사 결과</Text>
              <div className="video-repair-format-list">
                정밀 검사 결과가 있으면 복구 판단에 정밀 검사 결과를 우선 사용합니다.
              </div>
            </div>
            {repairBasisResult?.file.path && (
              <Button size="small" onClick={() => void handleRevealFile(repairBasisResult.file.path)}>
                파일 위치 열기
              </Button>
            )}
          </div>

          {(quickResult || deepResult) && (
            <div className="video-repair-inspection-history">
              {quickResult && (
                <div className="video-repair-inspection-history-item">
                  <Tag>빠른 검사</Tag>
                  <Text className="app-muted">{quickResult.summary}</Text>
                </div>
              )}
              {deepResult && (
                <div className="video-repair-inspection-history-item is-preferred">
                  <Tag color="processing">정밀 검사 · 복구 판단 기준</Tag>
                  <Text className="app-muted">{deepResult.summary}</Text>
                </div>
              )}
            </div>
          )}

          {repairBasisResult ? (
            <VideoInspectionResultView result={repairBasisResult} />
          ) : (
            <div className="video-repair-result-placeholder">
              <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="아직 실행한 검사가 없습니다." />
            </div>
          )}
            </Card>
          </>
        )}
      </div>
    </Card>
  );
}

export default VideoRepairPanel;
