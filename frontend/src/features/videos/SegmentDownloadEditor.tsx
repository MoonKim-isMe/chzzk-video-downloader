import { Alert, Button, Checkbox, Input, Modal, Space, Tag, Typography } from 'antd';
import { useRef, useState } from 'react';

import type { DownloadType } from '../../types/download';
import { formatSegmentTime, parseChapterList, validateSegmentRange } from './segmentRanges';
import type { SegmentRange } from './segmentRanges';

const { Text, Title } = Typography;

interface SegmentRow extends SegmentRange {
  id: number;
  selected: boolean;
}

interface SegmentDownloadEditorProps {
  duration: number;
  onRegister: (segments: SegmentRange[], downloadType: DownloadType, numberFileName: boolean) => Promise<void>;
}

function SegmentDownloadEditor({ duration, onRegister }: SegmentDownloadEditorProps) {
  const nextId = useRef(1);
  const [rawChapters, setRawChapters] = useState('');
  const [rows, setRows] = useState<SegmentRow[]>([]);
  const [error, setError] = useState('');
  const [importError, setImportError] = useState('');
  const [importOpen, setImportOpen] = useState(false);
  const [numberFileName, setNumberFileName] = useState(true);
  const [registering, setRegistering] = useState<DownloadType | null>(null);

  const applyChapters = () => {
    try {
      const parsed = parseChapterList(rawChapters, duration);
      setRows(parsed.map((range) => ({ ...range, id: nextId.current++, selected: true })));
      setImportError('');
      setImportOpen(false);
      setError('');
    } catch (cause) {
      setImportError(cause instanceof Error ? cause.message : '타임스탬프를 확인해 주세요.');
    }
  };

  const addRow = () => {
    setRows((current) => [...current, {
      id: nextId.current++,
      selected: true,
      title: '구간 ' + (current.length + 1),
      start: '00:00',
      end: duration > 0 ? formatSegmentTime(duration) : '',
    }]);
    setError('');
  };

  const updateRow = (id: number, field: keyof SegmentRange, value: string) => {
    setRows((current) => current.map((row) => row.id === id ? { ...row, [field]: value } : row));
  };

  const selectedRows = rows.filter((row) => row.selected);
  const invalidRows = selectedRows.filter((row) => validateSegmentRange(row, duration) !== null).length;
  const allSelected = rows.length > 0 && selectedRows.length === rows.length;
  const canDownload = selectedRows.length > 0 && invalidRows === 0;

  const registerSegments = async (downloadType: DownloadType) => {
    if (!canDownload || registering !== null) return;
    setRegistering(downloadType);
    setError('');
    try {
      await onRegister(
        selectedRows.map(({ title, start, end }) => ({ title, start, end })),
        downloadType,
        numberFileName,
      );
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '구간 작업을 등록하지 못했습니다.');
    } finally {
      setRegistering(null);
    }
  };

  return (
    <section className="segment-editor" aria-label="구간 다운로드 편집">
      <div className="segment-editor-heading">
        <div className="segment-editor-heading-content">
          <Title level={4} className="!mb-1">구간 다운로드</Title>
          <Text type="secondary" className="!text-[13px]">댓글 타임스탬프로 자동 생성하거나 직접 구간을 추가할 수 있습니다.</Text>
        </div>
        <div className="segment-editor-heading-import" aria-label="타임스탬프 일괄 입력">
          <div className="segment-editor-heading-import-label">
            <Text strong>타임스탬프 일괄 입력</Text>
            <Tag color="warning" className="!m-0">자동 생성 시 기존 구간을 대체합니다</Tag>
          </div>
          <Button type="primary" onClick={() => { setImportError(''); setImportOpen(true); }}>
            일괄 입력
          </Button>
        </div>
      </div>
      {error && (
        <div className="segment-editor-error">
          <Alert type="error" showIcon message={error} />
        </div>
      )}

      <div className="segment-editor-list-header">
        <div className="segment-editor-list-title">
          <Text strong>다운로드 구간</Text>
          <Tag color="green" className="!m-0">{selectedRows.length}/{rows.length}개 선택</Tag>
        </div>
        <Space size={4} wrap>
          <Button size="small" disabled={allSelected} onClick={() => setRows((current) => current.map((row) => ({ ...row, selected: true })))}>전체 선택</Button>
          <Button size="small" disabled={selectedRows.length === 0} onClick={() => setRows((current) => current.map((row) => ({ ...row, selected: false })))}>전체 해제</Button>
          <Button size="small" onClick={addRow}>+ 구간 추가</Button>
        </Space>
      </div>

      {rows.length === 0 ? (
        <Text type="secondary" className="segment-editor-empty">추가된 구간이 없습니다.</Text>
      ) : (
        <div className="segment-editor-list-scroll">
          <div className="segment-editor-list">
            <div className="segment-editor-grid segment-editor-grid-heading" aria-hidden="true">
              <Text type="secondary" className="!text-[13px]">선택</Text>
              <Text type="secondary" className="!text-[13px]">제목</Text>
              <Text type="secondary" className="!text-[13px]">시작</Text>
              <Text type="secondary" className="!text-[13px]">종료</Text>
              <span />
            </div>
            {rows.map((row, index) => {
              const rowError = row.selected ? validateSegmentRange(row, duration) : null;
              return (
                <div className={'segment-editor-grid segment-editor-row' + (row.selected ? '' : ' is-unselected')} key={row.id}>
                  <div className="segment-editor-row-select">
                    <Checkbox
                      checked={row.selected}
                      onChange={(event) => setRows((current) => current.map((item) => item.id === row.id ? { ...item, selected: event.target.checked } : item))}
                      aria-label={'구간 ' + (index + 1) + ' 다운로드 선택'}
                    />
                    <Text type="secondary" className="!text-[13px]">{index + 1}</Text>
                  </div>
                  <Input
                    size="small"
                    value={row.title}
                    maxLength={150}
                    onChange={(event) => updateRow(row.id, 'title', event.target.value)}
                    aria-label={'구간 ' + (index + 1) + ' 제목'}
                  />
                  <Input
                    size="small"
                    className="segment-editor-time-input"
                    value={row.start}
                    onChange={(event) => updateRow(row.id, 'start', event.target.value)}
                    aria-label={'구간 ' + (index + 1) + ' 시작 시간'}
                    suffix={
                      <button
                        type="button"
                        className="segment-editor-time-action"
                        onMouseDown={(event) => event.preventDefault()}
                        onClick={() => updateRow(row.id, 'start', '00:00')}
                        aria-label={'구간 ' + (index + 1) + ' 처음부터'}
                      >
                        처음부터
                      </button>
                    }
                  />
                  <Input
                    size="small"
                    className="segment-editor-time-input"
                    value={row.end}
                    onChange={(event) => updateRow(row.id, 'end', event.target.value)}
                    aria-label={'구간 ' + (index + 1) + ' 종료 시간'}
                    suffix={
                      <button
                        type="button"
                        className="segment-editor-time-action"
                        onMouseDown={(event) => event.preventDefault()}
                        disabled={!Number.isFinite(duration) || duration <= 0}
                        onClick={() => updateRow(row.id, 'end', formatSegmentTime(duration))}
                        aria-label={'구간 ' + (index + 1) + ' 끝까지'}
                      >
                        끝까지
                      </button>
                    }
                  />
                  <Button size="small" danger type="text" onClick={() => setRows((current) => current.filter((item) => item.id !== row.id))}>삭제</Button>
                  {rowError && <Text type="danger" className="segment-editor-row-error !text-[13px]">{rowError}</Text>}
                </div>
              );
            })}
          </div>
        </div>
      )}

      <div className="segment-editor-footer">
        <div className="segment-editor-footer-options">
          <Checkbox checked={numberFileName} onChange={(event) => setNumberFileName(event.target.checked)}>
            파일명에 번호 추가
          </Checkbox>
          <Text type="secondary" className="!text-[13px]">
            {numberFileName ? '예: 01. 영상1.mp4' : '예: 영상1.mp4'}
          </Text>
        </div>
        <div className="segment-editor-footer-actions">
          <Button
            type="primary"
            block
            disabled={!canDownload || registering === 'audio'}
            loading={registering === 'video'}
            onClick={() => void registerSegments('video')}
          >
            동영상 다운로드
          </Button>
          <Button
            block
            disabled={!canDownload || registering === 'video'}
            loading={registering === 'audio'}
            onClick={() => void registerSegments('audio')}
          >
            MP3 다운로드
          </Button>
        </div>
        <Text type="secondary" className="!text-[13px]">
          선택한 {selectedRows.length}개 구간을 목록 순서대로 다운로드합니다. 종료 시간은 숫자로 입력하거나 '끝까지' 버튼으로 설정합니다.
        </Text>
        {invalidRows > 0 && <Text type="danger" className="!text-[13px]">수정이 필요한 선택 구간 {invalidRows}개</Text>}
      </div>

      <Modal
        title={
          <Space size={8} wrap>
            <span>타임스탬프 일괄 입력</span>
            <Tag color="warning" className="!m-0">자동 생성 시 기존 구간을 대체합니다</Tag>
          </Space>
        }
        open={importOpen}
        onCancel={() => { setImportOpen(false); setImportError(''); }}
        onOk={applyChapters}
        okText="구간 자동 생성"
        cancelText="취소"
        destroyOnHidden
        width={560}
        className="segment-editor-import-modal"
      >
        <div className="segment-editor-modal-content">
          <Text type="secondary">댓글의 타임스탬프와 제목을 줄 단위로 붙여넣으세요.</Text>
          <Input.TextArea
            value={rawChapters}
            rows={8}
            onChange={(event) => setRawChapters(event.target.value)}
            placeholder={'2:59 영상 1\n6:54 영상 2\n7:52 영상 3'}
            aria-label="타임스탬프 일괄 입력"
          />
          {importError && <Alert type="error" showIcon message={importError} />}
        </div>
      </Modal>
    </section>
  );
}

export default SegmentDownloadEditor;
