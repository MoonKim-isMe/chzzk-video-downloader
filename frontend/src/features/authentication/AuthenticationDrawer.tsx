import {
  Alert,
  App as AntdApp,
  Button,
  Drawer,
  Form,
  Input,
  Skeleton,
  Switch,
  Typography,
} from 'antd';
import { useEffect, useState } from 'react';

import {
  getAuthenticationSettings,
  selectAuthenticationCookiesFile,
  updateAuthenticationSettings,
} from '../../lib/backend';
import type { AuthenticationSettings } from '../../types/authentication';
import CookiesTxtHelpDrawer from './CookiesTxtHelpDrawer';

const { Paragraph, Text, Title } = Typography;

interface AuthenticationDrawerProps {
  open: boolean;
  onClose: () => void;
  onAuthenticationUpdated: (settings: AuthenticationSettings) => void;
}

function AuthenticationDrawer({
  open,
  onClose,
  onAuthenticationUpdated,
}: AuthenticationDrawerProps) {
  const { message } = AntdApp.useApp();
  const [form] = Form.useForm<AuthenticationSettings>();
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [selectingFile, setSelectingFile] = useState(false);
  const [helpOpen, setHelpOpen] = useState(false);
  const enabled = Form.useWatch('enabled', form) ?? false;

  useEffect(() => {
    if (!open) return;
    let active = true;
    setLoading(true);
    getAuthenticationSettings()
      .then((settings) => {
        if (active) form.setFieldsValue(settings);
      })
      .catch((cause) => {
        if (active) message.error(cause instanceof Error ? cause.message : String(cause));
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [form, message, open]);

  const handleClose = () => {
    setHelpOpen(false);
    onClose();
  };

  const handleSelectCookiesFile = async () => {
    setSelectingFile(true);
    try {
      const currentFile = form.getFieldValue('cookiesFilePath') ?? '';
      const selected = await selectAuthenticationCookiesFile(currentFile);
      if (selected) {
        form.setFieldValue('cookiesFilePath', selected);
        await form.validateFields(['cookiesFilePath']);
      }
    } catch (cause) {
      message.error(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setSelectingFile(false);
    }
  };

  const handleSave = async () => {
    setSaving(true);
    try {
      const values = await form.validateFields();
      const updated = await updateAuthenticationSettings({
        ...values,
        cookiesFilePath: values.cookiesFilePath?.trim() ?? '',
      });
      form.setFieldsValue(updated);
      onAuthenticationUpdated(updated);
      message.success(updated.enabled ? '토큰 인증을 사용하도록 저장했습니다.' : '인증 사용을 해제했습니다.');
      handleClose();
    } catch (cause) {
      if (cause && typeof cause === 'object' && 'errorFields' in cause) return;
      message.error(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setSaving(false);
    }
  };

  return (
    <>
      <Drawer
        title="토큰 인증"
        placement="right"
        width={460}
        open={open}
        onClose={handleClose}
        destroyOnHidden
        footer={
          <div className="flex justify-end gap-2">
            <Button onClick={handleClose} disabled={saving}>취소</Button>
            <Button type="primary" loading={saving} disabled={loading} onClick={() => void handleSave()}>
              저장
            </Button>
          </div>
        }
      >
        {loading ? (
          <Skeleton active paragraph={{ rows: 6 }} />
        ) : (
          <Form form={form} layout="vertical" requiredMark={false}>
            <section className="settings-section">
              <Text className="app-eyebrow">AUTHENTICATION</Text>
              <Title level={5} className="!mb-1 !mt-1">cookies.txt 사용</Title>
              <Paragraph className="app-muted !mb-5 !text-xs">
                로그인이 필요한 영상에 사용할 cookies.txt를 선택합니다.
              </Paragraph>

              <Form.Item name="enabled" label="토큰 인증 사용" valuePropName="checked">
                <Switch checkedChildren="사용" unCheckedChildren="사용 안 함" />
              </Form.Item>

              <Form.Item
                name="cookiesFilePath"
                label="cookies.txt"
                dependencies={['enabled']}
                rules={[
                  ({ getFieldValue }) => ({
                    validator: async (_, value?: string) => {
                      if (getFieldValue('enabled') && !value?.trim()) {
                        throw new Error('cookies.txt 파일을 선택해 주세요.');
                      }
                    },
                  }),
                ]}
              >
                <Input
                  readOnly
                  disabled={!enabled}
                  placeholder="cookies.txt 파일을 선택해 주세요."
                  addonAfter={
                    <Button
                      type="text"
                      size="small"
                      disabled={!enabled}
                      loading={selectingFile}
                      onClick={() => void handleSelectCookiesFile()}
                    >
                      파일 선택
                    </Button>
                  }
                />
              </Form.Item>

              <Alert
                type="warning"
                showIcon
                className="!mb-2"
                message="갑자기 동작하지 않으면 세션이 만료된 것일 수 있습니다."
                description="cookies.txt를 다시 발급해 주세요."
              />

              <Button
                type="link"
                className="!h-auto !p-0 !text-xs"
                onClick={() => setHelpOpen(true)}
              >
                Cookies.txt 생성방법이 어려운 분은 여기를 눌러주세요
              </Button>

              <div className="mt-5 border-t border-[var(--app-border-soft)] pt-4">
                <Text strong className="!text-xs">주의사항</Text>
                <Paragraph className="app-muted !mb-0 !mt-1 !text-xs">
                  cookies.txt에는 로그인 세션 정보가 포함되어 있습니다. 다른 사람과 공유하지 마세요.
                </Paragraph>
              </div>
            </section>
          </Form>
        )}
      </Drawer>

      <CookiesTxtHelpDrawer open={helpOpen} onClose={() => setHelpOpen(false)} />
    </>
  );
}

export default AuthenticationDrawer;
