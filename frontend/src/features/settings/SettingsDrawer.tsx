import {
  Alert,
  App as AntdApp,
  Button,
  Drawer,
  Form,
  Input,
  InputNumber,
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
import type { AppSettings, DownloadResolution, OutputFormat } from '../../types/settings';

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

interface SettingsDrawerProps {
  open: boolean;
  onClose: () => void;
}

function SettingsDrawer({ open, onClose }: SettingsDrawerProps) {
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
      width={480}
      open={open}
      onClose={onClose}
      destroyOnHidden
      extra={
        <Text className="!text-xs !text-slate-500">
          Phase 6 Persistence
        </Text>
      }
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
        <Skeleton active paragraph={{ rows: 8 }} />
      ) : (
        <Form
          form={form}
          layout="vertical"
          requiredMark={false}
          className="space-y-2"
        >
          <div>
            <Title level={5} className="!mb-1">
              다운로드
            </Title>
            <Paragraph className="!mb-5 !text-xs !text-slate-500">
              Queue에 새로 추가하는 작업부터 아래 다운로드 옵션을 사용합니다.
            </Paragraph>
          </div>

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

          <Alert
            type="info"
            showIcon
            message="설정 적용 시점"
            description={
              <Space direction="vertical" size={2}>
                <Text className="!text-xs">
                  경로·해상도·포맷은 Queue 등록 시점에 고정됩니다.
                </Text>
                <Text className="!text-xs">
                  동시 다운로드 수는 저장 즉시 Scheduler에 적용됩니다.
                </Text>
              </Space>
            }
          />

          <Alert
            className="mt-3"
            type="success"
            showIcon
            message="설정은 앱 재시작 후에도 유지됩니다."
            description="저장한 설정은 SQLite에 보관되며 다음 실행 시 자동으로 복원됩니다."
          />
        </Form>
      )}
    </Drawer>
  );
}

export default SettingsDrawer;
