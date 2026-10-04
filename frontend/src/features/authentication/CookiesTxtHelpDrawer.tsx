import { Alert, Drawer, Typography } from 'antd';

import cookiesTxtGuideImage from '../../assets/cookies-txt-guide.webp';

const { Paragraph, Text, Title } = Typography;

interface CookiesTxtHelpDrawerProps {
  open: boolean;
  onClose: () => void;
}

function CookiesTxtHelpDrawer({ open, onClose }: CookiesTxtHelpDrawerProps) {
  return (
    <Drawer
      title="cookies.txt 생성 방법"
      placement="right"
      width={620}
      open={open}
      onClose={onClose}
      destroyOnHidden
    >
      <section className="settings-section">
        <Text className="app-eyebrow">GET COOKIES.TXT LOCALLY</Text>
        <Title level={5} className="!mb-1 !mt-1">CHZZK 쿠키 내보내기</Title>
        <Paragraph className="app-muted !mb-4 !text-xs">
          아래 화면처럼 Get cookies.txt LOCALLY에서 CHZZK 쿠키를 내보내면 됩니다.
        </Paragraph>

        <div className="overflow-hidden rounded-xl border border-[var(--app-border)] bg-white">
          <img
            src={cookiesTxtGuideImage}
            alt="Get cookies.txt LOCALLY에서 Export 버튼과 Netscape 형식을 선택하는 화면"
            className="block h-auto w-full"
            draggable={false}
          />
        </div>

        <Title level={5} className="!mb-2 !mt-5">사용 방법</Title>
        <ol className="!mb-5 list-decimal space-y-2 pl-5 text-sm leading-6">
          <li>
            CHZZK에 로그인한 뒤 영상 또는 라이브 페이지를 열고, 브라우저 우측 상단의
            <Text strong> Get cookies.txt LOCALLY</Text> 아이콘을 누릅니다.
          </li>
          <li>
            <Text strong>Export Format</Text>을 <Text code>Netscape</Text>로 선택합니다.
          </li>
          <li>
            위쪽 <Text strong>Export</Text>를 눌러 cookies.txt를 저장한 뒤 앱의 토큰 인증에서
            해당 파일을 선택합니다.
          </li>
        </ol>

        <Alert
          type="warning"
          showIcon
          message="주의사항"
          description={
            <>
              <Text code>Export All Cookies</Text>가 아니라 현재 CHZZK 페이지의 <Text strong>Export</Text>를
              사용해 주세요. cookies.txt는 로그인 세션 정보이므로 다른 사람과 공유하지 마세요.
            </>
          }
        />
      </section>
    </Drawer>
  );
}

export default CookiesTxtHelpDrawer;
