export type VideoInspectionMode = 'quick' | 'deep';

export type VideoDeepInspectionCPUProfile = 'low' | 'default' | 'high' | 'max';

export type VideoRepairWorkflowStatus =
  | 'idle'
  | 'ready'
  | 'inspecting'
  | 'inspected'
  | 'repairing'
  | 'repaired'
  | 'failed';

export type VideoInspectionStatus = 'normal' | 'warning' | 'damaged' | 'failed';

export type VideoRepairability =
  | 'not_needed'
  | 'lossless'
  | 'partial'
  | 'reencode'
  | 'impossible';

export type VideoInspectionHealth =
  | 'unknown'
  | 'normal'
  | 'warning'
  | 'damaged'
  | 'unavailable';

export type VideoRepairStrategy =
  | 'none'
  | 'compatibility_remux'
  | 'remux'
  | 'timestamp_remux'
  | 'partial'
  | 'truncate'
  | 'reencode'
  | 'unavailable';

export type VideoMoovAtomStatus = 'not_applicable' | 'present' | 'missing' | 'unknown';

export interface VideoRepairFileInfo {
  path: string;
  name: string;
  sizeBytes: number;
  modifiedUnixMilli: number;
  extension: string;
  container: string;
  containerSource: 'probe' | 'extension';
  formatName?: string;
  durationSeconds: number;
  metadataAvailable: boolean;
  metadataError?: string;
}

export interface VideoContainerInspection {
  status: VideoInspectionHealth;
  parseable: boolean;
  streamMetadataReadable: boolean;
  indexStatus: VideoInspectionHealth;
  moovAtomStatus: VideoMoovAtomStatus;
  errorCount: number;
}

export interface VideoStreamInspection {
  status: VideoInspectionHealth;
  present: boolean;
  codec?: string;
  profile?: string;
  width?: number;
  height?: number;
  fps?: number;
  durationSeconds?: number;
  decodeErrorCount: number;
  corruptFrameCount: number;
  corruptPacketCount: number;
}

export interface AudioStreamInspection {
  status: VideoInspectionHealth;
  present: boolean;
  codec?: string;
  channels?: number;
  sampleRate?: number;
  durationSeconds?: number;
  decodeErrorCount: number;
  corruptPacketCount: number;
}

export interface VideoTimestampInspection {
  status: VideoInspectionHealth;
  ptsErrorCount: number;
  dtsErrorCount: number;
  nonMonotonicDtsCount: number;
  jumpCount: number;
}

export interface VideoAVSyncInspection {
  status: VideoInspectionHealth;
  videoDurationSeconds?: number;
  audioDurationSeconds?: number;
  durationDifferenceSeconds?: number;
  startDifferenceSeconds?: number;
}

export interface VideoDamageRange {
  startSeconds: number;
  endSeconds: number;
  category: string;
  errorCount: number;
}

export interface VideoRepairRecommendation {
  strategy: VideoRepairStrategy;
  summary: string;
  qualityLoss: boolean;
  segmentLoss: boolean;
}

export interface VideoDiagnosticReferences {
  ffprobeLogPath?: string;
  ffmpegLogPath?: string;
}

export interface VideoInspectionResult {
  mode: VideoInspectionMode;
  status: VideoInspectionStatus;
  repairability: VideoRepairability;
  summary: string;
  inspectedAt: string;
  file: VideoRepairFileInfo;
  container: VideoContainerInspection;
  video: VideoStreamInspection;
  audio: AudioStreamInspection;
  timestamps: VideoTimestampInspection;
  avSync: VideoAVSyncInspection;
  firstErrorSeconds?: number;
  lastErrorSeconds?: number;
  lastHealthySeconds?: number;
  damageRanges: VideoDamageRange[];
  recommendation: VideoRepairRecommendation;
  deepInspectionRecommended: boolean;
  deepInspectionReason?: string;
  diagnostics: VideoDiagnosticReferences;
}

export interface VideoInspectionProgress {
  mode: VideoInspectionMode;
  path: string;
  percent: number;
  stage: string;
  sampleIndex?: number;
  sampleCount?: number;
  processedSeconds?: number;
  speed?: number;
  elapsedSeconds?: number;
}

export interface VideoRepairPlan {
  sourcePath: string;
  outputPath: string;
  strategy: VideoRepairStrategy;
  inspectionMode: VideoInspectionMode;
  sourceSizeBytes: number;
  sourceModifiedUnixMilli: number;
  requiredFreeBytes: number;
  summary: string;
  expectedTime: string;
  qualityLoss: boolean;
  segmentLoss: boolean;
  executable: boolean;
  normalizeTimestamps?: boolean;
  hasVideo?: boolean;
  hasAudio?: boolean;
  durationSeconds?: number;
  endSeconds?: number;
}

export interface VideoRepairProgress {
  sourcePath: string;
  outputPath: string;
  strategy: VideoRepairStrategy;
  percent: number;
  stage: string;
  processedSeconds?: number;
  speed?: number;
  elapsedSeconds?: number;
}

export interface VideoRepairResult {
  plan: VideoRepairPlan;
  outputFile: VideoRepairFileInfo;
  startedAt: string;
  completedAt: string;
  elapsedSeconds: number;
  repairLogPath?: string;
  autoInspection?: VideoInspectionResult;
  autoInspectionError?: string;
}

export interface VideoRepairSession {
  file?: VideoRepairFileInfo;
  inspectionMode: VideoInspectionMode;
  status: VideoRepairWorkflowStatus;
  completedInspectionModes: VideoInspectionMode[];
  hasInspectionResult: boolean;
  result?: VideoInspectionResult;
  inspectionResults: Partial<Record<VideoInspectionMode, VideoInspectionResult>>;
  repairPlan?: VideoRepairPlan;
  repairResult?: VideoRepairResult;
  error?: string;
}

export const createInitialVideoRepairSession = (): VideoRepairSession => ({
  inspectionMode: 'quick',
  status: 'idle',
  completedInspectionModes: [],
  hasInspectionResult: false,
  inspectionResults: {},
});

export const isVideoRepairBusy = (status: VideoRepairWorkflowStatus) =>
  status === 'inspecting' || status === 'repairing';

export const canRepairInspectionResult = (result?: VideoInspectionResult) =>
  Boolean(
    result &&
    result.status !== 'failed' &&
    result.repairability !== 'not_needed' &&
    result.repairability !== 'impossible',
  );

export const preferredVideoInspectionResult = (session: VideoRepairSession) =>
  session.inspectionResults.deep ?? session.inspectionResults.quick ?? session.result;
