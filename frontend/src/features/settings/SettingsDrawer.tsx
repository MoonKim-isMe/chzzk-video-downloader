import {
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
  Typography,
} from 'antd';
import { useEffect, useRef, useState } from 'react';

import {
  getSettings,
  selectDownloadDirectory,
  updateSettings,
} from '../../lib/backend';
import type {
  AppSettings,
  DownloadAcceleration,
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

const accelerationOptions: Array<{ label: string; value: DownloadAcceleration }> = [
  { label: '안정 (1)', value: 'stable' },
  { label: '기본 (2)', value: 'standard' },
  { label: '고속 (4)', value: 'fast' },
  { label: '초고속 (8)', value: 'ultra' },
];

const themeOptions: Array<{ label: string; value: ThemeMode }> = [
  { label: '☀ 라이트', value: 'light' },
  { label: '◐ 다크', value: 'dark' },
];

interface SettingsDrawerProps {
  open: boolean;
  onClose: () => void;
  onThemePreview: (theme: ThemeMode) => void;
  onSettingsUpdated: (settings: AppSettings) => void;
  checkingForUpdates: boolean;
  onCheckForUpdates: () => void;
}

function SettingsDrawer({
  open,
  onClose,
  onThemePreview,
  onSettingsUpdated,
  checkingForUpdates,
  onCheckForUpdates,
}: SettingsDrawerProps) {
  const { message } = AntdApp.useApp();
  const [form] = Form.useForm<AppSettings>();
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [selectingDirectory, setSelectingDirectory] = useState(false);
  const persistedTheme = useRef<ThemeMode | undefined>(undefined);

  useEffect(() => {
    if (!open) {
      return;
    }

    let active = true;
    persistedTheme.current = undefined;
    setLoading(true);

    getSettings()
      .then((settings) => {
        if (active) {
          persistedTheme.current = settings.theme;
          form.setFieldsValue(settings);
          onThemePreview(settings.theme);
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
  }, [form, message, onThemePreview, open]);

  const handleClose = () => {
    if (persistedTheme.current) {
      onThemePreview(persistedTheme.current);
    }
    onClose();
  };

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
      persistedTheme.current = updated.theme;
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
      onClose={handleClose}
      destroyOnHidden
      footer={
        <div className="flex justify-end gap-2">
          <Button onClick={handleClose} disabled={saving}>
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
              <Segmented
                block
                options={themeOptions}
                onChange={(value) => onThemePreview(value as ThemeMode)}
              />
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

            <Form.Item name="downloadAcceleration" label="다운로드 가속" rules={[{ required: true }]}>
              <Select options={accelerationOptions} />
            </Form.Item>

            <Form.Item
              name="downloadRateLimitMBps"
              label="다운로드 속도 제한"
              extra="각 다운로드 기준입니다. 0이면 속도를 제한하지 않습니다."
              rules={[
                { required: true, message: '다운로드 속도 제한을 입력해 주세요.' },
                { type: 'number', min: 0, message: '0 이상의 값을 입력해 주세요.' },
              ]}
            >
              <InputNumber min={0} step={1} precision={2} addonAfter="MB/s" className="!w-full" />
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

          <Divider className="!my-6" />

          <section className="settings-section">
            <Text className="app-eyebrow">UPDATE</Text>
            <Title level={5} className="!mb-1 !mt-1">
              업데이트
            </Title>
            <Paragraph className="app-muted !mb-4 !text-xs">
              GitHub Releases에서 새 버전이 있는지 확인합니다.
            </Paragraph>
            <Button loading={checkingForUpdates} onClick={onCheckForUpdates}>
              업데이트 확인
            </Button>
          </section>

        </Form>
      )}
    </Drawer>
  );
}

export default SettingsDrawer;
