import {
  App as AntdApp,
  Button,
  Card,
  ConfigProvider,
  Empty,
  Layout,
  Tooltip,
  theme as antdTheme,
} from 'antd';
import { useCallback, useEffect, useMemo, useState } from 'react';

import SavedChannels from './components/SavedChannels';
import ChannelSearchTab from './features/channels/ChannelSearchTab';
import DownloadPanel from './features/downloads/DownloadPanel';
import SettingsDrawer from './features/settings/SettingsDrawer';
import ChannelVideoList from './features/videos/ChannelVideoList';
import VideoUrlTab from './features/videos/VideoUrlTab';
import {
  getDownloadTasks,
  getSavedChannels,
  getSettings,
  removeSavedChannel,
  saveChannel,
  startDownload,
} from './lib/backend';
import { mergeDownloadTaskSnapshot, upsertDownloadTask } from './lib/downloadTasks';
import { onDownloadState } from './lib/runtime';
import type { Channel } from './types/channel';
import type { DownloadTask, DownloadTaskStatus } from './types/download';
import type { AppSettings, ThemeMode } from './types/settings';
import type { Video } from './types/video';

const { Header, Content } = Layout;

type AppTab = 'search' | 'url' | 'downloads';
type ChannelPanelTab = 'search' | 'bookmarks';

const navigationItems: Array<{ key: AppTab; label: string }> = [
  { key: 'search', label: '채널 검색' },
  { key: 'url', label: 'URL 직접 입력' },
  { key: 'downloads', label: '다운로드' },
];

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
  onSettingsUpdated: (settings: AppSettings) => void;
}

function AppContent({ themeMode, onSettingsUpdated }: AppContentProps) {
  const { message } = AntdApp.useApp();
  const [activeTab, setActiveTab] = useState<AppTab>('search');
  const [channelPanelTab, setChannelPanelTab] = useState<ChannelPanelTab>('search');
  const [savedChannels, setSavedChannels] = useState<Channel[]>([]);
  const [selectedChannel, setSelectedChannel] = useState<Channel>();
  const [downloadTasks, setDownloadTasks] = useState<DownloadTask[]>([]);
  const [settingsOpen, setSettingsOpen] = useState(false);

  useEffect(() => {
    getSavedChannels()
      .then(setSavedChannels)
      .catch((cause) => {
        message.error(cause instanceof Error ? cause.message : String(cause));
      });
  }, [message]);

  useEffect(() => {
    let active = true;
    const unsubscribe = onDownloadState((task) => {
      if (active) {
        setDownloadTasks((current) => upsertDownloadTask(current, task));
      }
    });

    getDownloadTasks()
      .then((snapshot) => {
        if (active) {
          setDownloadTasks((current) => mergeDownloadTaskSnapshot(current, snapshot));
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
          task.status === 'running' ? '다운로드를 시작했습니다.' : '다운로드 Queue에 추가했습니다.',
        );
      } catch (cause) {
        message.error(cause instanceof Error ? cause.message : String(cause));
      }
    },
    [message],
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
          {channelPanelTab === 'search' ? (
            <ChannelSearchTab
              savedChannelIds={savedChannelIds}
              selectedChannelId={selectedChannel?.channelId}
              onSelect={selectChannel}
              onSave={handleSave}
              onRemove={handleRemove}
            />
          ) : (
            <SavedChannels
              channels={savedChannels}
              selectedChannelId={selectedChannel?.channelId}
              onSelect={selectChannel}
              onRemove={handleRemove}
            />
          )}
        </div>
      </Card>

      <div className="channel-vod-column">
        {selectedChannel ? (
          <ChannelVideoList
            key={selectedChannel.channelId}
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

  const content = activeTab === 'search'
    ? searchWorkspace
    : activeTab === 'url'
      ? (
          <div className="app-scroll-view">
            <Card bordered={false} className="app-panel">
              <VideoUrlTab
                activeDownloadStatusByVideoNo={activeDownloadStatusByVideoNo}
                onQueueVideo={handleQueueVideo}
              />
            </Card>
          </div>
        )
      : (
          <div className="app-scroll-view">
            <DownloadPanel tasks={downloadTasks} />
          </div>
        );

  return (
    <Layout className="app-shell" data-theme={themeMode}>
      <Header className="app-header">
        <div className="app-brand" title="CHZZK Video Downloader">
          <span className="app-brand-mark">C</span>
          <span className="app-brand-name">CHZZK Video Downloader</span>
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
          <Tooltip title="설정" placement="bottom">
            <Button
              aria-label="설정 열기"
              type="text"
              shape="circle"
              className="settings-icon-button"
              icon={<SettingsIcon />}
              onClick={() => setSettingsOpen(true)}
            />
          </Tooltip>
        </div>
      </Header>

      <Content className="app-content">
        <main className="app-content-inner">{content}</main>
      </Content>

      <SettingsDrawer
        open={settingsOpen}
        onClose={() => setSettingsOpen(false)}
        onSettingsUpdated={onSettingsUpdated}
      />
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
      <AntdApp>
        <AppContent
          themeMode={themeMode}
          onSettingsUpdated={(settings) => setThemeMode(settings.theme)}
        />
      </AntdApp>
    </ConfigProvider>
  );
}

export default App;
