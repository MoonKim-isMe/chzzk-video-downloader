import {
  App as AntdApp,
  Card,
  ConfigProvider,
  Empty,
  Layout,
  Space,
  Tabs,
  Tag,
  Typography,
} from 'antd';
import { useCallback, useEffect, useMemo, useState } from 'react';
import type { ReactNode } from 'react';

import SavedChannels from './components/SavedChannels';
import ChannelSearchTab from './features/channels/ChannelSearchTab';
import ChannelUrlTab from './features/channels/ChannelUrlTab';
import DownloadPanel from './features/downloads/DownloadPanel';
import ChannelVideoList from './features/videos/ChannelVideoList';
import {
  getDownloadTasks,
  getSavedChannels,
  removeSavedChannel,
  saveChannel,
  startDownload,
} from './lib/backend';
import { mergeDownloadTaskSnapshot, upsertDownloadTask } from './lib/downloadTasks';
import { onDownloadState } from './lib/runtime';
import type { Channel } from './types/channel';
import type { DownloadTask, DownloadTaskStatus } from './types/download';
import type { Video } from './types/video';

const { Header, Content } = Layout;
const { Text, Title } = Typography;

function AppContent() {
  const { message } = AntdApp.useApp();
  const [savedChannels, setSavedChannels] = useState<Channel[]>([]);
  const [selectedChannelId, setSelectedChannelId] = useState<string>();
  const [downloadTasks, setDownloadTasks] = useState<DownloadTask[]>([]);

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

  const selectedChannel = useMemo(
    () => savedChannels.find((channel) => channel.channelId === selectedChannelId),
    [savedChannels, selectedChannelId],
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
    setSelectedChannelId(channel.channelId);
  };

  const handleSave = async (channel: Channel) => {
    try {
      const channels = await saveChannel(channel);
      setSavedChannels(channels);
      setSelectedChannelId(channel.channelId);
      message.success(`${channel.channelName} 채널을 저장했습니다.`);
    } catch (cause) {
      message.warning(cause instanceof Error ? cause.message : String(cause));
    }
  };

  const handleRemove = async (channelId: string) => {
    try {
      const channels = await removeSavedChannel(channelId);
      setSavedChannels(channels);
      if (selectedChannelId === channelId) {
        setSelectedChannelId(undefined);
      }
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

  const channelWorkspace = (content: ReactNode) => (
    <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_320px]">
      <div className="min-w-0 space-y-6">
        <Card className="border-slate-800 bg-slate-900/80">{content}</Card>
        {selectedChannel ? (
          <ChannelVideoList
            key={selectedChannel.channelId}
            channel={selectedChannel}
            activeDownloadStatusByVideoNo={activeDownloadStatusByVideoNo}
            onQueueVideo={handleQueueVideo}
          />
        ) : (
          <Card className="border-slate-800 bg-slate-900/80">
            <Empty description="저장 채널에서 채널을 선택하면 VOD 목록을 표시합니다." />
          </Card>
        )}
      </div>
      <SavedChannels
        channels={savedChannels}
        selectedChannelId={selectedChannelId}
        onSelect={selectChannel}
        onRemove={handleRemove}
      />
    </div>
  );

  return (
    <Layout className="min-h-screen bg-slate-950">
      <Header className="flex h-16 items-center justify-between border-b border-slate-800 bg-slate-950 px-6">
        <Space size={12}>
          <Title level={4} className="!m-0 !text-slate-100">
            CHZZK Video Downloader
          </Title>
          <Tag>Phase 4-C</Tag>
        </Space>
        <Text className="!text-slate-400">Wails v2</Text>
      </Header>

      <Content className="overflow-auto p-6">
        <Tabs
          defaultActiveKey="search"
          items={[
            {
              key: 'search',
              label: '채널 검색',
              children: channelWorkspace(
                <ChannelSearchTab savedChannelIds={savedChannelIds} onSave={handleSave} />,
              ),
            },
            {
              key: 'url',
              label: 'URL 직접 입력',
              children: channelWorkspace(
                <ChannelUrlTab savedChannelIds={savedChannelIds} onSave={handleSave} />,
              ),
            },
            {
              key: 'downloads',
              label: '다운로드',
              children: <DownloadPanel tasks={downloadTasks} />,
            },
          ]}
        />
      </Content>
    </Layout>
  );
}

function App() {
  return (
    <ConfigProvider>
      <AntdApp>
        <AppContent />
      </AntdApp>
    </ConfigProvider>
  );
}

export default App;
