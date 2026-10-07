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
import { mergeDownloadTaskSnapshot, upsertDownloadTask } from './lib/downloadTasks';
import { onDownloadState } from './lib/runtime';
import type { AuthenticationSettings } from './types/authentication';
import type { Channel } from './types/channel';
import type { DownloadTask, DownloadTaskStatus } from './types/download';
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
  const [channelPanelTab, setChannelPanelTab] = useState<ChannelPanelTab>('search');
  const [savedChannels, setSavedChannels] = useState<Channel[]>([]);
  const [selectedChannel, setSelectedChannel] = useState<Channel>();
  const [downloadTasks, setDownloadTasks] = useState<DownloadTask[]>([]);
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
          setUpdateModalOpen(true);
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

  const authenticationEnabled = authenticationSettings?.enabled ?? false;
  const authenticationButtonLabel = authenticationEnabled ? '토큰 인증' : '인증 미사용';

  const savedChannelIds = useMemo(
    () => new Set(savedChannels.map((channel) => channel.channelId)),
    [savedChannels],
  );

  const activeDownloadStatusByVideoNo = useMemo(() => {
    const active = new Map<number, DownloadTaskStatus>();
    downloadTasks.forEach((task) => {
      if (task.status === 'queued' || task.status === 'running') {
        active.set(task.videoNo, task.status);
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

  const handleQueueVideo = useCallback(
    async (video: Video) => {
      try {
        const task = await startDownload({
          videoNo: video.videoNo,
          videoTitle: video.videoTitle,
          channelName: video.channel.channelName,
          thumbnailImageUrl: video.thumbnailImageUrl,
          url: video.videoUrl,
          outputDir: '',
        });
        setDownloadTasks((current) => upsertDownloadTask(current, task));
        message.success(
          task.status === 'running' ? '다운로드를 시작했습니다.' : '다운로드 목록에 추가했습니다.',
        );
      } catch (cause) {
        message.error(cause instanceof Error ? cause.message : String(cause));
      }
    },
    [message],
  );

  const handleDownloadTaskRemoved = useCallback((taskId: string) => {
    hiddenDownloadTaskIds.current.add(taskId);
    setDownloadTasks((current) => current.filter((task) => task.taskId !== taskId));
  }, []);

  const handleDownloadTaskRecovered = useCallback(
    (previousTaskId: string, task: DownloadTask) => {
      hiddenDownloadTaskIds.current.add(previousTaskId);
      setDownloadTasks((current) =>
        upsertDownloadTask(
          current.filter((item) => item.taskId !== previousTaskId),
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
          />
        ) : (
          <Card bordered={false} className="app-panel channel-vod-empty">
            <Empty description="검색 또는 북마크에서 채널을 선택해 주세요." />
          </Card>
        )}
      </div>
    </div>
  );

  const secondaryContent = activeTab === 'url'
    ? (
        <VideoUrlTab
          activeDownloadStatusByVideoNo={activeDownloadStatusByVideoNo}
          onQueueVideo={handleQueueVideo}
        />
      )
    : activeTab === 'downloads'
      ? (
          <DownloadPanel
            tasks={downloadTasks}
            onTaskRemoved={handleDownloadTaskRemoved}
            onTaskRecovered={handleDownloadTaskRecovered}
          />
        )
      : activeTab === 'repair'
        ? <VideoRepairPanel />
        : null;

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
              title="GitHub Release 페이지 열기"
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
          {secondaryContent && (
            <div className="app-main-tab-view">
              {secondaryContent}
            </div>
          )}
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
        title="새 버전이 있습니다"
        open={updateModalOpen && Boolean(availableUpdate?.updateAvailable)}
        onCancel={() => setUpdateModalOpen(false)}
        footer={[
          <Button key="close" onClick={() => setUpdateModalOpen(false)}>
            닫기
          </Button>,
          <Button
            key="release"
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
            릴리즈 페이지 열기
          </Button>,
        ]}
      >
        <p>새 버전을 사용할 수 있습니다. GitHub Releases에서 변경사항과 다운로드 파일을 확인할 수 있습니다.</p>
        <p>
          <strong>현재 버전</strong> v{availableUpdate?.currentVersion}
        </p>
        <p>
          <strong>최신 버전</strong> v{availableUpdate?.latestVersion}
        </p>
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
