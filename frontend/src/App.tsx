import {
  App as AntdApp,
  Button,
  Card,
  ConfigProvider,
  Empty,
  Layout,
  Modal,
  Tooltip,
  theme as antdTheme,
} from 'antd';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import ReleaseNotes from './components/ReleaseNotes';
import SavedChannels from './components/SavedChannels';
import AuthenticationDrawer from './features/authentication/AuthenticationDrawer';
import ChannelSearchTab from './features/channels/ChannelSearchTab';
import DownloadPanel from './features/downloads/DownloadPanel';
import VideoRepairPanel from './features/repair/VideoRepairPanel';
import SettingsDrawer from './features/settings/SettingsDrawer';
import ChannelVideoList from './features/videos/ChannelVideoList';
import VideoUrlTab from './features/videos/VideoUrlTab';
import {
  checkForUpdates,
  getAuthenticationSettings,
  getDownloadTasks,
  getSavedChannels,
  getSettings,
  openReleasePage,
  removeSavedChannel,
  saveChannel,
  startDownload,
} from './lib/backend';
import {
  getRemainingDownloadCount,
  isStartupDownloadRequest,
  mergeDownloadTaskSnapshot,
  upsertDownloadTask,
} from './lib/downloadTasks';
import { getPostponedUpdateVersion, postponeUpdateVersion, shouldOpenUpdateModal } from './lib/updateNotifications';
import { onDownloadState } from './lib/runtime';
import type { AuthenticationSettings } from './types/authentication';
import type { Channel } from './types/channel';
import type { DownloadTask, DownloadTaskStatus, DownloadType } from './types/download';
import { parseSegmentTime } from './features/videos/segmentRanges';
import type { SegmentRange } from './features/videos/segmentRanges';
import type { AppSettings, ThemeMode } from './types/settings';
import type { UpdateInfo } from './types/update';
import type { Video } from './types/video';

const { Header, Content } = Layout;

type AppTab = 'search' | 'url' | 'downloads' | 'repair';
type ChannelPanelTab = 'search' | 'bookmarks';

const navigationItems: Array<{ key: AppTab; label: string }> = [
  { key: 'search', label: '채널 검색' },
  { key: 'url', label: 'URL 직접 입력' },
  { key: 'downloads', label: '다운로드' },
  { key: 'repair', label: '동영상 복구' },
];

const messageTopOffset = 84;

function AuthenticationIcon() {
  return (
    <svg aria-hidden="true" viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <rect x="5" y="10" width="14" height="10" rx="2" />
      <path d="M8 10V7a4 4 0 0 1 8 0v3" />
      <path d="M12 14v2" />
    </svg>
  );
}

function SettingsIcon() {
  return (
    <svg
      aria-hidden="true"
      viewBox="0 0 24 24"
      width="18"
      height="18"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <path d="M12 15.5a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7Z" />
      <path d="M19.4 15a1.7 1.7 0 0 0 .34 1.87l.06.06-2.83 2.83-.06-.06a1.7 1.7 0 0 0-1.87-.34 1.7 1.7 0 0 0-1.04 1.56V21h-4v-.08A1.7 1.7 0 0 0 8.96 19.36a1.7 1.7 0 0 0-1.87.34l-.06.06-2.83-2.83.06-.06A1.7 1.7 0 0 0 4.6 15a1.7 1.7 0 0 0-1.56-1.04H3v-4h.04A1.7 1.7 0 0 0 4.6 8.92a1.7 1.7 0 0 0-.34-1.87L4.2 6.99l2.83-2.83.06.06a1.7 1.7 0 0 0 1.87.34A1.7 1.7 0 0 0 10 3h4a1.7 1.7 0 0 0 1.04 1.56 1.7 1.7 0 0 0 1.87-.34l.06-.06 2.83 2.83-.06.06a1.7 1.7 0 0 0-.34 1.87A1.7 1.7 0 0 0 20.96 10H21v4h-.04A1.7 1.7 0 0 0 19.4 15Z" />
    </svg>
  );
}

interface AppContentProps {
  themeMode: ThemeMode;
  onThemePreview: (theme: ThemeMode) => void;
  onSettingsUpdated: (settings: AppSettings) => void;
}

function AppContent({ themeMode, onThemePreview, onSettingsUpdated }: AppContentProps) {
  const { message } = AntdApp.useApp();
  const [activeTab, setActiveTab] = useState<AppTab>('search');
  const [segmentRequest, setSegmentRequest] = useState<{ video: Video; sequence: number }>();
  const [channelPanelTab, setChannelPanelTab] = useState<ChannelPanelTab>('search');
  const [savedChannels, setSavedChannels] = useState<Channel[]>([]);
  const [selectedChannel, setSelectedChannel] = useState<Channel>();
  const [downloadTasks, setDownloadTasks] = useState<DownloadTask[]>([]);
  // A session request keeps its identity through prepared activation and retries.
  const [requestIdByTask, setRequestIdByTask] = useState<Map<string, string>>(() => new Map());
  const [completedRequestIds, setCompletedRequestIds] = useState<Set<string>>(() => new Set());
  const hiddenDownloadTaskIds = useRef(new Set<string>());
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [authenticationOpen, setAuthenticationOpen] = useState(false);
  const [authenticationSettings, setAuthenticationSettings] = useState<AuthenticationSettings>();
  const [availableUpdate, setAvailableUpdate] = useState<UpdateInfo>();
  const [updateModalOpen, setUpdateModalOpen] = useState(false);
  const [checkingForUpdates, setCheckingForUpdates] = useState(false);
  const startupUpdateCheckStarted = useRef(false);

  const handleCheckForUpdates = useCallback(
    async (notifyWhenCurrent: boolean) => {
      setCheckingForUpdates(true);
      try {
        const result = await checkForUpdates();
        if (result.updateAvailable) {
          setSettingsOpen(false);
          setAvailableUpdate(result);
          setUpdateModalOpen(shouldOpenUpdateModal(result.latestVersion, getPostponedUpdateVersion(), notifyWhenCurrent));
        } else {
          setAvailableUpdate(undefined);
          if (notifyWhenCurrent) {
            message.success(`현재 최신 버전(v${result.currentVersion})을 사용 중입니다.`);
          }
        }
      } catch (cause) {
        if (notifyWhenCurrent) {
          message.warning(cause instanceof Error ? cause.message : String(cause));
        }
      } finally {
        setCheckingForUpdates(false);
      }
    },
    [message],
  );

  const handlePostponeUpdate = useCallback(() => {
    if (availableUpdate?.updateAvailable) {
      postponeUpdateVersion(availableUpdate.latestVersion);
    }
    setUpdateModalOpen(false);
  }, [availableUpdate]);

  useEffect(() => {
    if (startupUpdateCheckStarted.current) {
      return;
    }
    startupUpdateCheckStarted.current = true;
    void handleCheckForUpdates(false);
  }, [handleCheckForUpdates]);

  useEffect(() => {
    getSavedChannels()
      .then(setSavedChannels)
      .catch((cause) => {
        message.error(cause instanceof Error ? cause.message : String(cause));
      });
  }, [message]);

  useEffect(() => {
    let active = true;
    getAuthenticationSettings()
      .then((settings) => {
        if (active) setAuthenticationSettings(settings);
      })
      .catch((cause) => {
        if (active) message.error(cause instanceof Error ? cause.message : String(cause));
      });
    return () => {
      active = false;
    };
  }, [message]);

  useEffect(() => {
    let active = true;
    const unsubscribe = onDownloadState((task) => {
      if (!active) {
        return;
      }
      if (task.status === 'cancelled') {
        hiddenDownloadTaskIds.current.add(task.taskId);
        setDownloadTasks((current) => current.filter((item) => item.taskId !== task.taskId));
        return;
      }
      if (hiddenDownloadTaskIds.current.has(task.taskId)) {
        return;
      }
      setDownloadTasks((current) => upsertDownloadTask(current, task));
    });

    getDownloadTasks()
      .then((snapshot) => {
        if (active) {
          setRequestIdByTask((current) => {
            const next = new Map(current);
            snapshot.forEach((task) => {
              if (isStartupDownloadRequest(task) && !next.has(task.taskId)) {
                next.set(task.taskId, task.taskId);
              }
            });
            return next;
          });
          const visibleSnapshot = snapshot.filter(
            (task) => task.status !== 'cancelled' && !hiddenDownloadTaskIds.current.has(task.taskId),
          );
          setDownloadTasks((current) => mergeDownloadTaskSnapshot(current, visibleSnapshot));
        }
      })
      .catch((cause) => {
        if (active) {
          message.error(cause instanceof Error ? cause.message : String(cause));
        }
      });

    return () => {
      active = false;
      unsubscribe();
    };
  }, [message]);

  useEffect(() => {
    const completed = downloadTasks
      .filter((task) => task.status === 'completed')
      .map((task) => requestIdByTask.get(task.taskId))
      .filter((requestId): requestId is string => Boolean(requestId));

    if (completed.length > 0) {
      setCompletedRequestIds((current) => {
        if (completed.every((requestId) => current.has(requestId))) {
          return current;
        }
        return new Set([...current, ...completed]);
      });
    }
  }, [downloadTasks, requestIdByTask]);

  const totalRequestCount = useMemo(
    () => new Set(requestIdByTask.values()).size,
    [requestIdByTask],
  );
  const remainingDownloadCount = useMemo(
    () => getRemainingDownloadCount(downloadTasks),
    [downloadTasks],
  );

  const authenticationEnabled = authenticationSettings?.enabled ?? false;
  const authenticationButtonLabel = authenticationEnabled ? '토큰 인증' : '인증 미사용';

  const savedChannelIds = useMemo(
    () => new Set(savedChannels.map((channel) => channel.channelId)),
    [savedChannels],
  );

  const activeDownloadStatusByVideoNo = useMemo(() => {
    const active = new Map<string, DownloadTaskStatus>();
    downloadTasks.forEach((task) => {
      if (!task.segment && (task.status === 'queued' || task.status === 'running')) {
        active.set(`${task.videoNo}:${task.downloadType === 'audio' ? 'audio' : 'video'}`, task.status);
      }
    });
    return active;
  }, [downloadTasks]);

  const selectChannel = (channel: Channel) => {
    setSelectedChannel(channel);
  };

  const handleSave = async (channel: Channel) => {
    try {
      const channels = await saveChannel(channel);
      setSavedChannels(channels);
      message.success(`${channel.channelName} 채널을 북마크했습니다.`);
    } catch (cause) {
      message.warning(cause instanceof Error ? cause.message : String(cause));
    }
  };

  const handleRemove = async (channelId: string) => {
    try {
      const channels = await removeSavedChannel(channelId);
      setSavedChannels(channels);
    } catch (cause) {
      message.error(cause instanceof Error ? cause.message : String(cause));
    }
  };

  const handleOpenSegments = useCallback((video: Video) => {
    setSegmentRequest((current) => ({ video, sequence: (current?.sequence ?? 0) + 1 }));
    setActiveTab('url');
  }, []);

  const handleQueueVideo = useCallback(
    async (video: Video, downloadType: DownloadType = 'video') => {
      try {
        const task = await startDownload({
          videoNo: video.videoNo,
          downloadType,
          videoTitle: video.videoTitle,
          channelName: video.channel.channelName,
          thumbnailImageUrl: video.thumbnailImageUrl,
          url: video.videoUrl,
          outputDir: '',
        });
        setDownloadTasks((current) => upsertDownloadTask(current, task));
        setRequestIdByTask((current) =>
          current.has(task.taskId) ? current : new Map(current).set(task.taskId, task.taskId),
        );
        message.success(
          task.status === 'running' ? `${downloadType === 'audio' ? 'MP3' : '동영상'} 다운로드를 시작했습니다.` : '다운로드 목록에 추가했습니다.',
        );
      } catch (cause) {
        message.error(cause instanceof Error ? cause.message : String(cause));
      }
    },
    [message],
  );

  const handleQueueSegments = useCallback(
    async (video: Video, segments: SegmentRange[], downloadType: DownloadType, numberFileName: boolean) => {
      const requests = segments.map((segment, index) => {
        const startSeconds = parseSegmentTime(segment.start);
        const endSeconds = parseSegmentTime(segment.end);
        if (startSeconds === null || endSeconds === null) {
          throw new Error('구간 시간을 확인해 주세요.');
        }
        return {
          downloadType,
          videoNo: video.videoNo,
          videoTitle: video.videoTitle,
          channelName: video.channel.channelName,
          thumbnailImageUrl: video.thumbnailImageUrl,
          url: video.videoUrl,
          outputDir: '',
          segment: {
            title: segment.title.trim(),
            position: index + 1,
            numberFileName,
            startSeconds,
            endSeconds,
          },
        };
      });
      // Register clips in visible order. Parallel requests can reach the Go
      // queue out of order and may reserve the wrong numbered filename.
      let successfulCount = 0;
      const failures: string[] = [];
      for (const request of requests) {
        try {
          const task = await startDownload(request);
          successfulCount += 1;
          setDownloadTasks((current) => upsertDownloadTask(current, task));
          setRequestIdByTask((current) =>
            current.has(task.taskId) ? current : new Map(current).set(task.taskId, task.taskId),
          );
        } catch (cause) {
          failures.push(cause instanceof Error ? cause.message : String(cause));
        }
      }
      if (failures.length) {
        throw new Error(
          `${successfulCount}개 등록, ${failures.length}개 실패: ${failures[0]}`,
        );
      }
      message.success(`${successfulCount}개 구간을 다운로드 목록에 추가했습니다.`);
    },
    [message],
  );

  const handleDownloadTaskRemoved = useCallback((taskId: string) => {
    hiddenDownloadTaskIds.current.add(taskId);
    setDownloadTasks((current) => current.filter((task) => task.taskId !== taskId));
  }, []);

  const handleDownloadTaskRecovered = useCallback(
    (previousTaskId: string, task: DownloadTask) => {
      // Prepared jobs activate under the same taskId; never hide their
      // progress events. Failure retries use a new ID and hide the old one.
      if (previousTaskId !== task.taskId) {
        hiddenDownloadTaskIds.current.add(previousTaskId);
      }
      setRequestIdByTask((current) => {
        const next = new Map(current);
        const requestId = next.get(previousTaskId) ?? task.taskId;
        next.delete(previousTaskId);
        next.set(task.taskId, requestId);
        return next;
      });
      setDownloadTasks((current) =>
        upsertDownloadTask(
          current.filter((item) => previousTaskId === task.taskId || item.taskId !== previousTaskId),
          task,
        ),
      );
    },
    [],
  );

  const searchWorkspace = (
    <div className="channel-workspace">
      <Card bordered={false} className="app-panel channel-sidebar-panel">
        <div className="channel-sidebar-tabs" role="tablist" aria-label="채널 탐색">
          <button
            type="button"
            role="tab"
            aria-selected={channelPanelTab === 'search'}
            className={`channel-sidebar-tab ${channelPanelTab === 'search' ? 'is-active' : ''}`}
            onClick={() => setChannelPanelTab('search')}
          >
            검색
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={channelPanelTab === 'bookmarks'}
            className={`channel-sidebar-tab ${channelPanelTab === 'bookmarks' ? 'is-active' : ''}`}
            onClick={() => setChannelPanelTab('bookmarks')}
          >
            북마크
            <span className="channel-sidebar-tab-count">{savedChannels.length}</span>
          </button>
        </div>

        <div className="channel-sidebar-body">
          <div
            className="channel-sidebar-tab-view"
            hidden={channelPanelTab !== 'search'}
          >
            <ChannelSearchTab
              savedChannelIds={savedChannelIds}
              selectedChannelId={selectedChannel?.channelId}
              onSelect={selectChannel}
              onSave={handleSave}
              onRemove={handleRemove}
            />
          </div>
          <div
            className="channel-sidebar-tab-view"
            hidden={channelPanelTab !== 'bookmarks'}
          >
            <SavedChannels
              channels={savedChannels}
              selectedChannelId={selectedChannel?.channelId}
              onSelect={selectChannel}
              onRemove={handleRemove}
            />
          </div>
        </div>
      </Card>

      <div className="channel-vod-column">
        {selectedChannel ? (
          <ChannelVideoList
            key={selectedChannel.channelId}
            authenticationEnabled={authenticationEnabled}
            channel={selectedChannel}
            activeDownloadStatusByVideoNo={activeDownloadStatusByVideoNo}
            onQueueVideo={handleQueueVideo}
            onOpenSegments={handleOpenSegments}
          />
        ) : (
          <Card bordered={false} className="app-panel channel-vod-empty">
            <Empty description="검색 또는 북마크에서 채널을 선택해 주세요." />
          </Card>
        )}
      </div>
    </div>
  );

  return (
    <Layout className="app-shell" data-theme={themeMode}>
      <Header className="app-header">
        <div className="app-brand" title="CHZZK Video Downloader">
          <span className="app-brand-mark">C</span>
          <span className="app-brand-name">CHZZK Video Downloader</span>
          {availableUpdate?.updateAvailable && (
            <button
              type="button"
              className="app-update-tag"
              title="업데이트 다운로드 페이지 열기"
              onClick={() => {
                void openReleasePage(availableUpdate.releaseUrl).catch((cause) => {
                  message.error(cause instanceof Error ? cause.message : String(cause));
                });
              }}
            >
              새 업데이트가 있습니다
            </button>
          )}
        </div>

        <nav className="app-navigation" aria-label="주요 메뉴">
          {navigationItems.map((item) => (
            <button
              key={item.key}
              type="button"
              className={`app-nav-button ${activeTab === item.key ? 'is-active' : ''}`}
              aria-current={activeTab === item.key ? 'page' : undefined}
              onClick={() => setActiveTab(item.key)}
            >
              {item.label}
              {item.key === 'downloads' && (
                <span
                  className={`app-nav-remaining-tag ${remainingDownloadCount === 0 ? 'is-empty' : ''}`}
                >
                  남은 항목 {remainingDownloadCount}
                </span>
              )}
            </button>
          ))}
        </nav>

        <div className="app-header-actions">
          <Tooltip
            title={authenticationEnabled ? `${authenticationButtonLabel} 사용 중` : '인증'}
            placement="bottom"
          >
            <Button
              aria-label="인증 열기"
              type="default"
              className={`authentication-header-button ${authenticationEnabled ? 'is-active' : ''}`}
              icon={<AuthenticationIcon />}
              onClick={() => setAuthenticationOpen(true)}
            >
              <span className="authentication-button-label">{authenticationButtonLabel}</span>
              {authenticationEnabled && <span className="authentication-status-dot" aria-hidden="true" />}
            </Button>
          </Tooltip>
          <Tooltip title="설정" placement="bottom">
            <Button
              aria-label="설정 열기"
              type="default"
              className="settings-icon-button"
              icon={<SettingsIcon />}
              onClick={() => setSettingsOpen(true)}
            />
          </Tooltip>
        </div>
      </Header>

      <Content className="app-content">
        <main className="app-content-inner">
          <div
            className="app-main-tab-view"
            hidden={activeTab !== 'search'}
          >
            {searchWorkspace}
          </div>
          <div
            className="app-main-tab-view"
            hidden={activeTab !== 'url'}
          >
            <VideoUrlTab
              activeDownloadStatusByVideoNo={activeDownloadStatusByVideoNo}
              onQueueVideo={handleQueueVideo}
              segmentRequest={segmentRequest}
              onQueueSegments={handleQueueSegments}
            />
          </div>
          {activeTab === 'downloads' && (
            <div className="app-main-tab-view">
              <DownloadPanel
                tasks={downloadTasks}
                onTaskRemoved={handleDownloadTaskRemoved}
                onTaskRecovered={handleDownloadTaskRecovered}
                completedRequestCount={completedRequestIds.size}
                totalRequestCount={totalRequestCount}
              />
            </div>
          )}
          <div
            className="app-main-tab-view"
            hidden={activeTab !== 'repair'}
          >
            <VideoRepairPanel />
          </div>
        </main>
      </Content>

      <AuthenticationDrawer
        open={authenticationOpen}
        onClose={() => setAuthenticationOpen(false)}
        onAuthenticationUpdated={setAuthenticationSettings}
      />

      <SettingsDrawer
        open={settingsOpen}
        onClose={() => setSettingsOpen(false)}
        onThemePreview={onThemePreview}
        onSettingsUpdated={onSettingsUpdated}
        checkingForUpdates={checkingForUpdates}
        onCheckForUpdates={() => void handleCheckForUpdates(true)}
      />

      <Modal
        className="app-update-modal"
        title="새 버전이 있습니다"
        width={620}
        open={updateModalOpen && Boolean(availableUpdate?.updateAvailable)}
        onCancel={handlePostponeUpdate}
        footer={[
          <Button key="later" onClick={handlePostponeUpdate}>
            나중에
          </Button>,
          <Button
            key="download"
            type="primary"
            onClick={() => {
              if (!availableUpdate?.releaseUrl) {
                return;
              }
              void openReleasePage(availableUpdate.releaseUrl)
                .then(() => setUpdateModalOpen(false))
                .catch((cause) => {
                  message.error(cause instanceof Error ? cause.message : String(cause));
                });
            }}
          >
            다운로드 페이지 열기
          </Button>,
        ]}
      >
        <p>새 버전을 사용할 수 있습니다. 업데이트 내용을 확인해 주세요.</p>
        <div className="app-update-version-details">
          <div><span>현재 버전</span><strong>v{availableUpdate?.currentVersion}</strong></div>
          <div><span>최신 버전</span><strong>v{availableUpdate?.latestVersion}</strong></div>
        </div>
        <h3 className="app-update-notes-title">업데이트 내용</h3>
        <ReleaseNotes body={availableUpdate?.releaseNotes ?? ''} />
      </Modal>
    </Layout>
  );
}

function App() {
  const [themeMode, setThemeMode] = useState<ThemeMode>('dark');

  useEffect(() => {
    let active = true;
    getSettings()
      .then((settings) => {
        if (active) {
          setThemeMode(settings.theme);
        }
      })
      .catch(() => {
        // Backend settings are validated separately; keep the safe dark fallback here.
      });

    return () => {
      active = false;
    };
  }, []);

  useEffect(() => {
    document.documentElement.dataset.theme = themeMode;
  }, [themeMode]);

  return (
    <ConfigProvider
      theme={{
        algorithm: themeMode === 'dark' ? antdTheme.darkAlgorithm : antdTheme.defaultAlgorithm,
        token: {
          colorPrimary: '#00c471',
          borderRadius: 12,
          borderRadiusLG: 16,
          fontFamily:
            'Pretendard, Inter, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif',
        },
      }}
    >
      <AntdApp message={{ top: messageTopOffset }}>
        <AppContent
          themeMode={themeMode}
          onThemePreview={setThemeMode}
          onSettingsUpdated={(settings) => setThemeMode(settings.theme)}
        />
      </AntdApp>
    </ConfigProvider>
  );
}

export default App;
