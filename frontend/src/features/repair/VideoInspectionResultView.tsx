import {
  Alert,
  Collapse,
  Descriptions,
  Space,
  Tag,
  Typography,
} from 'antd';

import type {
  VideoDamageRange,
  VideoInspectionHealth,
  VideoInspectionResult,
  VideoInspectionStatus,
  VideoRepairStrategy,
  VideoRepairability,
} from '../../types/videoRepair';

const { Text } = Typography;

const statusMeta: Record<VideoInspectionStatus, { label: string; color: string; alert: 'success' | 'warning' | 'error' }> = {
  normal: { label: '정상', color: 'success', alert: 'success' },
  warning: { label: '경고', color: 'warning', alert: 'warning' },
  damaged: { label: '손상 감지', color: 'error', alert: 'error' },
  failed: { label: '검사 실패', color: 'default', alert: 'error' },
};

const healthLabels: Record<VideoInspectionHealth, string> = {
  unknown: '확인 전',
  normal: '정상',
  warning: '경고',
  damaged: '손상',
  unavailable: '확인 불가',
};

const repairabilityLabels: Record<VideoRepairability, string> = {
  not_needed: '복구 불필요',
  lossless: '무손실 복구 가능',
  partial: '부분 복구 가능',
  reencode: '재인코딩 필요',
  impossible: '복구 불가능',
};

const strategyLabels: Record<VideoRepairStrategy, string> = {
  none: '복구 불필요',
  compatibility_remux: '편집 호환성 복구',
  remux: 'Remux',
  timestamp_remux: 'Timestamp 정규화 + Remux',
  partial: '손상 데이터 제외 후 복구',
  truncate: '정상 구간까지 부분 복구',
  reencode: '재인코딩',
  unavailable: '복구 방법 없음',
};

function formatDuration(seconds?: number) {
  if (seconds === undefined || !Number.isFinite(seconds) || seconds < 0) return '확인 불가';
  const totalMilliseconds = Math.round(seconds * 1000);
  const totalSeconds = Math.floor(totalMilliseconds / 1000);
  const milliseconds = totalMilliseconds % 1000;
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const remaining = totalSeconds % 60;
  const base = [hours, minutes, remaining].map((value) => String(value).padStart(2, '0')).join(':');
  return milliseconds > 0 ? `${base}.${String(milliseconds).padStart(3, '0')}` : base;
}

function formatFileSize(bytes: number) {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const index = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  const value = bytes / (1024 ** index);
  return `${value.toLocaleString('ko-KR', { maximumFractionDigits: index >= 3 ? 2 : 1 })} ${units[index]}`;
}

function formatDifference(seconds?: number) {
  if (seconds === undefined || !Number.isFinite(seconds)) return '확인 불가';
  const sign = seconds > 0 ? '+' : '';
  return `${sign}${seconds.toFixed(3)}초`;
}

function yesNo(value: boolean) {
  return value ? '가능' : '불가';
}

function healthTag(status: VideoInspectionHealth) {
  const color = status === 'normal'
    ? 'success'
    : status === 'warning'
      ? 'warning'
      : status === 'damaged'
        ? 'error'
        : 'default';
  return <Tag color={color}>{healthLabels[status]}</Tag>;
}

function DamageRanges({ ranges }: { ranges: VideoDamageRange[] }) {
  if (ranges.length === 0) {
    return <Text className="app-muted">기록된 손상 구간이 없습니다.</Text>;
  }

  return (
    <div className="video-repair-damage-list">
      {ranges.map((range, index) => (
        <div
          key={`${range.startSeconds}-${range.endSeconds}-${index}`}
          className="video-repair-damage-row"
        >
          <div>
            <Text strong>{formatDuration(range.startSeconds)} ~ {formatDuration(range.endSeconds)}</Text>
            <div className="video-repair-detail-note">{range.category || '분류되지 않은 오류'}</div>
          </div>
          <Tag>{range.errorCount.toLocaleString()}건</Tag>
        </div>
      ))}
    </div>
  );
}

interface VideoInspectionResultViewProps {
  result: VideoInspectionResult;
}

function VideoInspectionResultView({ result }: VideoInspectionResultViewProps) {
  const overall = statusMeta[result.status];
  const damageRanges = result.damageRanges ?? [];
  const diagnosticCount = Number(Boolean(result.diagnostics?.ffprobeLogPath))
    + Number(Boolean(result.diagnostics?.ffmpegLogPath));

  return (
    <div className="video-repair-result">
      <Alert
        showIcon
        type={overall.alert}
        message={
          <Space wrap size={8}>
            <span>{overall.label}</span>
            <Tag color={overall.color}>{repairabilityLabels[result.repairability]}</Tag>
          </Space>
        }
        description={result.summary || result.recommendation.summary}
      />

      {result.mode === 'quick' && (
        <Alert
          showIcon
          type="info"
          message="빠른 검사 결과 안내"
          description={result.deepInspectionRecommended && result.deepInspectionReason
            ? result.deepInspectionReason
            : '빠른 검사는 파일 전체 프레임을 모두 디코딩하지 않으므로 영상 중간의 일부 손상을 발견하지 못할 수 있습니다. 재생 이상이 의심되면 정밀 검사를 진행해 주세요.'}
        />
      )}

      <div className="video-repair-result-summary">
        <div className="video-repair-result-stat">
          <span>검사 방식</span>
          <strong>{result.mode === 'quick' ? '빠른 검사' : '정밀 검사'}</strong>
        </div>
        <div className="video-repair-result-stat">
          <span>복구 가능성</span>
          <strong>{repairabilityLabels[result.repairability]}</strong>
        </div>
        <div className="video-repair-result-stat">
          <span>권장 복구</span>
          <strong>{strategyLabels[result.recommendation.strategy]}</strong>
        </div>
        <div className="video-repair-result-stat">
          <span>정밀 검사</span>
          <strong>{result.deepInspectionRecommended ? '권장' : '선택'}</strong>
        </div>
        <div className="video-repair-result-stat">
          <span>마지막 정상 위치</span>
          <strong>{formatDuration(result.lastHealthySeconds)}</strong>
        </div>
      </div>

      {result.recommendation.summary && (
        <div className="video-repair-recommendation">
          <div>
            <Text strong>권장 복구 방식</Text>
            <div className="video-repair-detail-note">{result.recommendation.summary}</div>
          </div>
          <Space wrap>
            <Tag color={result.recommendation.qualityLoss ? 'warning' : 'success'}>
              {result.recommendation.qualityLoss ? '화질 손실 가능' : '화질 손실 없음'}
            </Tag>
            <Tag color={result.recommendation.segmentLoss ? 'warning' : 'success'}>
              {result.recommendation.segmentLoss ? '구간 손실 가능' : '구간 손실 없음'}
            </Tag>
          </Space>
        </div>
      )}

      <Collapse
        className="video-repair-detail-collapse"
        items={[
          {
            key: 'file-container',
            label: '파일 및 컨테이너',
            children: (
              <Descriptions size="small" column={2}>
                <Descriptions.Item label="파일명">{result.file.name}</Descriptions.Item>
                <Descriptions.Item label="파일 크기">{formatFileSize(result.file.sizeBytes)}</Descriptions.Item>
                <Descriptions.Item label="컨테이너">{result.file.container}</Descriptions.Item>
                <Descriptions.Item label="재생 시간">{formatDuration(result.file.durationSeconds)}</Descriptions.Item>
                <Descriptions.Item label="컨테이너 상태">{healthTag(result.container.status)}</Descriptions.Item>
                <Descriptions.Item label="컨테이너 파싱">{yesNo(result.container.parseable)}</Descriptions.Item>
                <Descriptions.Item label="스트림 메타데이터">{yesNo(result.container.streamMetadataReadable)}</Descriptions.Item>
                <Descriptions.Item label="인덱스 상태">{healthTag(result.container.indexStatus)}</Descriptions.Item>
                <Descriptions.Item label="moov atom">{result.container.moovAtomStatus}</Descriptions.Item>
                <Descriptions.Item label="컨테이너 오류">{result.container.errorCount.toLocaleString()}건</Descriptions.Item>
              </Descriptions>
            ),
          },
          {
            key: 'streams',
            label: '영상 및 오디오 스트림',
            children: (
              <div className="video-repair-detail-grid">
                <div>
                  <div className="video-repair-detail-title">영상 {healthTag(result.video.status)}</div>
                  <Descriptions size="small" column={1}>
                    <Descriptions.Item label="코덱">{result.video.present ? (result.video.codec || '확인 불가') : '영상 스트림 없음'}</Descriptions.Item>
                    <Descriptions.Item label="해상도">
                      {result.video.width && result.video.height ? `${result.video.width} × ${result.video.height}` : '확인 불가'}
                    </Descriptions.Item>
                    <Descriptions.Item label="FPS">{result.video.fps ? result.video.fps.toFixed(3) : '확인 불가'}</Descriptions.Item>
                    <Descriptions.Item label="재생 시간">{formatDuration(result.video.durationSeconds)}</Descriptions.Item>
                    <Descriptions.Item label="디코딩 오류">{result.video.decodeErrorCount.toLocaleString()}건</Descriptions.Item>
                    <Descriptions.Item label="손상 프레임">{result.video.corruptFrameCount.toLocaleString()}건</Descriptions.Item>
                    <Descriptions.Item label="손상 패킷">{result.video.corruptPacketCount.toLocaleString()}건</Descriptions.Item>
                  </Descriptions>
                </div>
                <div>
                  <div className="video-repair-detail-title">오디오 {healthTag(result.audio.status)}</div>
                  <Descriptions size="small" column={1}>
                    <Descriptions.Item label="코덱">{result.audio.present ? (result.audio.codec || '확인 불가') : '오디오 스트림 없음'}</Descriptions.Item>
                    <Descriptions.Item label="채널">{result.audio.channels || '확인 불가'}</Descriptions.Item>
                    <Descriptions.Item label="샘플레이트">{result.audio.sampleRate ? `${result.audio.sampleRate.toLocaleString()} Hz` : '확인 불가'}</Descriptions.Item>
                    <Descriptions.Item label="재생 시간">{formatDuration(result.audio.durationSeconds)}</Descriptions.Item>
                    <Descriptions.Item label="디코딩 오류">{result.audio.decodeErrorCount.toLocaleString()}건</Descriptions.Item>
                    <Descriptions.Item label="손상 패킷">{result.audio.corruptPacketCount.toLocaleString()}건</Descriptions.Item>
                  </Descriptions>
                </div>
              </div>
            ),
          },
          {
            key: 'timestamps-sync',
            label: '타임스탬프 및 A/V Sync',
            children: (
              <div className="video-repair-detail-grid">
                <div>
                  <div className="video-repair-detail-title">타임스탬프 {healthTag(result.timestamps.status)}</div>
                  <Descriptions size="small" column={1}>
                    <Descriptions.Item label="PTS 오류">{result.timestamps.ptsErrorCount.toLocaleString()}건</Descriptions.Item>
                    <Descriptions.Item label="DTS 오류">{result.timestamps.dtsErrorCount.toLocaleString()}건</Descriptions.Item>
                    <Descriptions.Item label="Non-monotonous DTS">{result.timestamps.nonMonotonicDtsCount.toLocaleString()}건</Descriptions.Item>
                    <Descriptions.Item label="시간 점프">{result.timestamps.jumpCount.toLocaleString()}건</Descriptions.Item>
                  </Descriptions>
                </div>
                <div>
                  <div className="video-repair-detail-title">A/V Sync {healthTag(result.avSync.status)}</div>
                  <Descriptions size="small" column={1}>
                    <Descriptions.Item label="영상 길이">{formatDuration(result.avSync.videoDurationSeconds)}</Descriptions.Item>
                    <Descriptions.Item label="오디오 길이">{formatDuration(result.avSync.audioDurationSeconds)}</Descriptions.Item>
                    <Descriptions.Item label="길이 차이">{formatDifference(result.avSync.durationDifferenceSeconds)}</Descriptions.Item>
                    <Descriptions.Item label="시작 시점 차이">{formatDifference(result.avSync.startDifferenceSeconds)}</Descriptions.Item>
                  </Descriptions>
                </div>
              </div>
            ),
          },
          {
            key: 'damage',
            label: `손상 위치 ${damageRanges.length > 0 ? `(${damageRanges.length})` : ''}`,
            children: (
              <div>
                <Descriptions size="small" column={3} className="video-repair-damage-summary">
                  <Descriptions.Item label="최초 오류">{formatDuration(result.firstErrorSeconds)}</Descriptions.Item>
                  <Descriptions.Item label="마지막 오류">{formatDuration(result.lastErrorSeconds)}</Descriptions.Item>
                  <Descriptions.Item label="마지막 정상 위치">{formatDuration(result.lastHealthySeconds)}</Descriptions.Item>
                </Descriptions>
                <DamageRanges ranges={damageRanges} />
              </div>
            ),
          },
          {
            key: 'diagnostics',
            label: '기술 상세 및 진단 정보',
            children: (
              <div className="video-repair-diagnostics">
                <Descriptions size="small" column={2}>
                  <Descriptions.Item label="검사 시각">{new Date(result.inspectedAt).toLocaleString()}</Descriptions.Item>
                  <Descriptions.Item label="진단 로그">{diagnosticCount > 0 ? `${diagnosticCount}개 기록됨` : '기록 없음'}</Descriptions.Item>
                  <Descriptions.Item label="컨테이너 원본 형식">{result.file.formatName || '확인 불가'}</Descriptions.Item>
                  <Descriptions.Item label="검사 상태">{overall.label}</Descriptions.Item>
                </Descriptions>
                <Text className="video-repair-detail-note">
                  FFmpeg/FFprobe의 원본 stderr/stdout은 이 화면에 직접 표시하지 않고 별도 진단 로그로 보관합니다.
                </Text>
              </div>
            ),
          },
        ]}
      />
    </div>
  );
}

export default VideoInspectionResultView;
