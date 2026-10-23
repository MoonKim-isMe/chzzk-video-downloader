export interface LiveStatus {
 channelId: string;
 channelName: string;
 liveId: number;
 liveTitle: string;
 openDate: string;
 isLive: boolean;
}
export interface LiveSubscription {
 channelId: string;
 channelName: string;
 enabled: boolean;
 lastError?: string;
}
export interface LiveSession {
 id: string;
 channelId: string;
 channelName: string;
 liveId: number;
 title: string;
 directory: string;
 status: 'recording' | 'finalizing' | 'completed' | 'failed' | 'interrupted';
 startedAt: string;
 finishedAt?: string;
 finalPath?: string;
 error?: string;
 availableSeconds: number;
}
export interface LiveClip {
 id: string;
 sessionId: string;
 title: string;
 startSeconds: number;
 endSeconds: number;
 status: 'queued' | 'running' | 'completed' | 'failed';
 filePath?: string;
 error?: string;
}
export interface LiveState {
 pollSeconds: number;
 maxConcurrent: number;
 subscriptions: LiveSubscription[];
 sessions: LiveSession[];
 clips: LiveClip[];
}
