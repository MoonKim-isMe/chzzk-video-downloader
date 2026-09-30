import { App as AntdApp, Card, ConfigProvider, Layout, Space, Tag, Typography } from 'antd';

const { Header, Content } = Layout;
const { Paragraph, Text, Title } = Typography;

const foundations = [
  'Wails v2 + Go backend',
  'React 19 + TypeScript + Vite',
  'Tailwind CSS v4 + Ant Design v5',
  'Windows-first desktop application',
];

function App() {
  return (
    <ConfigProvider>
      <AntdApp>
        <Layout className="min-h-screen bg-slate-950">
          <Header className="flex h-16 items-center justify-between border-b border-slate-800 bg-slate-950 px-6">
            <Space size={12}>
              <Title level={4} className="!m-0 !text-slate-100">
                CHZZK Video Downloader
              </Title>
              <Tag>Phase 0</Tag>
            </Space>
            <Text className="!text-slate-400">Wails v2</Text>
          </Header>

          <Content className="flex flex-1 items-center justify-center p-8">
            <Card className="w-full max-w-2xl border-slate-800 bg-slate-900/80 shadow-2xl">
              <Space direction="vertical" size={20} className="w-full">
                <div>
                  <Title level={2} className="!mb-2 !text-slate-100">
                    프로젝트 기반 구성이 준비되었습니다.
                  </Title>
                  <Paragraph className="!mb-0 !text-slate-400">
                    다운로드 기능을 추가하기 전에 데스크톱 런타임과 프론트엔드 기반을 고정합니다.
                  </Paragraph>
                </div>

                <div className="grid gap-3 sm:grid-cols-2">
                  {foundations.map((item) => (
                    <div key={item} className="rounded-lg border border-slate-800 bg-slate-950/70 px-4 py-3">
                      <Text className="!text-slate-200">{item}</Text>
                    </div>
                  ))}
                </div>
              </Space>
            </Card>
          </Content>
        </Layout>
      </AntdApp>
    </ConfigProvider>
  );
}

export default App;
