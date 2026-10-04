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
      onClose();
    } catch (cause) {
      if (cause && typeof cause === 'object' && 'errorFields' in cause) return;
      message.error(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Drawer
      title="토큰 인증"
      placement="right"
      width={460}
      open={open}
      onClose={onClose}
      destroyOnHidden
      footer={
        <div className="flex justify-end gap-2">
          <Button onClick={onClose} disabled={saving}>취소</Button>
          <Button type="primary" loading={saving} disabled={loading} onClick={() => void handleSave()}>
            저장
          </Button>
        </div>
      }
    >
      {loading ? (
        <Skeleton active paragraph={{ rows: 7 }} />
      ) : (
        <Form form={form} layout="vertical" requiredMark={false}>
          <section className="settings-section">
            <Text className="app-eyebrow">AUTHENTICATION</Text>
            <Title level={5} className="!mb-1 !mt-1">cookies.txt 사용</Title>
            <Paragraph className="app-muted !mb-5 !text-xs">
              로그인 또는 계정 권한이 필요한 영상을 다운로드할 때 cookies.txt 파일의 로그인 세션을 사용합니다.
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
              message="cookies.txt에는 로그인 세션 정보가 포함되어 있습니다."
              description="계정 권한이 있는 콘텐츠에만 사용해 주세요. 앱은 파일 내용을 별도로 저장하지 않고 선택한 파일 경로만 저장합니다."
            />
          </section>
        </Form>
      )}
    </Drawer>
  );
}

export default AuthenticationDrawer;
