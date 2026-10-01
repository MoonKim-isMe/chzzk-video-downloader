import {
  Alert,
  App as AntdApp,
  Button,
  Divider,
  Drawer,
  Form,
  Input,
  InputNumber,
  Segmented,
  Select,
  Skeleton,
  Space,
  Typography,
} from 'antd';
import { useEffect, useState } from 'react';

import {
  getSettings,
  selectDownloadDirectory,
  updateSettings,
} from '../../lib/backend';
import type {
  AppSettings,
  DownloadResolution,
  OutputFormat,
  ThemeMode,
} from '../../types/settings';

const { Paragraph, Text, Title } = Typography;

const resolutionOptions: Array<{ label: string; value: DownloadResolution }> = [
  { label: '최고 화질', value: 'best' },
  { label: '2160p 이하', value: '2160p' },
  { label: '1440p 이하', value: '1440p' },
  { label: '1080p 이하', value: '1080p' },
  { label: '720p 이하', value: '720p' },
];

const formatOptions: Array<{ label: string; value: OutputFormat }> = [
  { label: 'MP4', value: 'mp4' },
  { label: 'MKV', value: 'mkv' },
  { label: 'WebM', value: 'webm' },
];

const themeOptions: Array<{ label: string; value: ThemeMode }> = [
  { label: '☀ 라이트', value: 'light' },
  { label: '◐ 다크', value: 'dark' },
];

interface SettingsDrawerProps {
  open: boolean;
  onClose: () => void;
  onSettingsUpdated: (settings: AppSettings) => void;
}

function SettingsDrawer({ open, onClose, onSettingsUpdated }: SettingsDrawerProps) {
  const { message } = AntdApp.useApp();
  const [form] = Form.useForm<AppSettings>();
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [selectingDirectory, setSelectingDirectory] = useState(false);

  useEffect(() => {
    if (!open) {
      return;
    }

    let active = true;
    setLoading(true);

    getSettings()
      .then((settings) => {
        if (active) {
          form.setFieldsValue(settings);
        }
      })
      .catch((cause) => {
        if (active) {
          message.error(cause instanceof Error ? cause.message : String(cause));
        }
      })
      .finally(() => {
        if (active) {
          setLoading(false);
        }
      });

    return () => {
      active = false;
    };
  }, [form, message, open]);

  const handleSelectDirectory = async () => {
    setSelectingDirectory(true);
    try {
      const currentDirectory = form.getFieldValue('downloadDir') ?? '';
      const selected = await selectDownloadDirectory(currentDirectory);
      if (selected) {
        form.setFieldValue('downloadDir', selected);
        await form.validateFields(['downloadDir']);
      }
    } catch (cause) {
      message.error(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setSelectingDirectory(false);
    }
  };

  const handleSave = async () => {
    setSaving(true);
    try {
      const values = await form.validateFields();
      const updated = await updateSettings({
        ...values,
        downloadDir: values.downloadDir.trim(),
      });
      form.setFieldsValue(updated);
      onSettingsUpdated(updated);
      message.success('설정을 저장했습니다.');
      onClose();
    } catch (cause) {
      if (cause && typeof cause === 'object' && 'errorFields' in cause) {
        return;
      }
      message.error(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Drawer
      title="설정"
      placement="right"
      width={460}
      open={open}
      onClose={onClose}
      destroyOnHidden
      footer={
        <div className="flex justify-end gap-2">
          <Button onClick={onClose} disabled={saving}>
            취소
          </Button>
          <Button type="primary" loading={saving} disabled={loading} onClick={() => void handleSave()}>
            저장
          </Button>
        </div>
      }
    >
      {loading ? (
        <Skeleton active paragraph={{ rows: 10 }} />
      ) : (
        <Form form={form} layout="vertical" requiredMark={false}>
          <section className="settings-section">
            <Text className="app-eyebrow">APPEARANCE</Text>
            <Title level={5} className="!mb-1 !mt-1">
              화면
            </Title>
            <Paragraph className="app-muted !mb-4 !text-xs">
              사용 환경에 맞게 앱의 밝기를 선택할 수 있습니다.
            </Paragraph>

            <Form.Item name="theme" label="테마" rules={[{ required: true }]}>
              <Segmented block options={themeOptions} />
            </Form.Item>
          </section>

          <Divider className="!my-6" />

          <section className="settings-section">
            <Text className="app-eyebrow">DOWNLOAD</Text>
            <Title level={5} className="!mb-1 !mt-1">
              다운로드
            </Title>
            <Paragraph className="app-muted !mb-5 !text-xs">
              새로 Queue에 추가하는 작업부터 아래 옵션을 사용합니다.
            </Paragraph>

            <Form.Item
              name="downloadDir"
              label="다운로드 폴더"
              rules={[
                {
                  validator: async (_, value?: string) => {
                    if (!value?.trim()) {
                      throw new Error('다운로드 폴더를 선택해 주세요.');
                    }
                    if (value.includes('\0')) {
                      throw new Error('다운로드 폴더에 사용할 수 없는 문자가 포함되어 있습니다.');
                    }
                  },
                },
              ]}
            >
              <Input
                readOnly
                placeholder="다운로드 폴더를 선택해 주세요."
                addonAfter={
                  <Button
                    type="text"
                    size="small"
                    loading={selectingDirectory}
                    onClick={() => void handleSelectDirectory()}
                  >
                    폴더 선택
                  </Button>
                }
              />
            </Form.Item>

            <Form.Item name="resolution" label="해상도" rules={[{ required: true }]}>
              <Select options={resolutionOptions} />
            </Form.Item>

            <Form.Item name="outputFormat" label="출력 포맷" rules={[{ required: true }]}>
              <Select options={formatOptions} />
            </Form.Item>

            <Form.Item
              name="maxConcurrentDownloads"
              label="동시 다운로드 수"
              rules={[
                { required: true, message: '동시 다운로드 수를 입력해 주세요.' },
                { type: 'number', min: 1, max: 8, message: '1~8 사이의 값을 입력해 주세요.' },
              ]}
            >
              <InputNumber min={1} max={8} precision={0} className="!w-full" />
            </Form.Item>
          </section>

          <Alert
            className="mt-2"
            type="info"
            showIcon
            message="설정 적용"
            description={
              <Space direction="vertical" size={2}>
                <Text className="!text-xs">
                  테마는 저장 즉시 전체 화면에 적용됩니다.
                </Text>
                <Text className="!text-xs">
                  경로·해상도·포맷은 새 Queue 작업부터, 동시 다운로드 수는 Scheduler에 즉시 적용됩니다.
                </Text>
              </Space>
            }
          />

          <Alert
            className="mt-3"
            type="success"
            showIcon
            message="설정은 자동으로 보관됩니다."
            description="SQLite에 저장되며 앱을 다시 실행해도 그대로 유지됩니다."
          />
        </Form>
      )}
    </Drawer>
  );
}

export default SettingsDrawer;
