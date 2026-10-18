import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Alert,
  Button,
  Form,
  Input,
  InputNumber,
  Modal,
  Progress,
  Radio,
  Select,
  Table,
  Tag,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';

import { useCleanIP } from '@/api/queries/useCleanIP';
import type { HostRecord } from '@/api/queries/useHostsQuery';
import type { CleanIPResult } from '@/generated/zod';
import type { InboundOption } from '@/schemas/client';

interface CleanIPScanModalProps {
  open: boolean;
  hosts: HostRecord[];
  inboundOptions: InboundOption[];
  onClose: () => void;
}

const KNOWN_REASONS = ['timeout', 'refused', 'certificate', 'reset', 'blocked', 'failed'];

export default function CleanIPScanModal({
  open,
  hosts,
  inboundOptions,
  onClose,
}: CleanIPScanModalProps) {
  const { t } = useTranslation();
  const { status, results, start, apply } = useCleanIP(open);
  const [targets, setTargets] = useState('');
  const [port, setPort] = useState(443);
  const [sni, setSni] = useState('');
  const [samples, setSamples] = useState(3);
  const [picked, setPicked] = useState<CleanIPResult | null>(null);
  const [destination, setDestination] = useState<string>();
  const [mode, setMode] = useState<'replace' | 'add'>('replace');

  const running = status?.running ?? false;
  const percent = status && status.total > 0 ? Math.round((status.done / status.total) * 100) : 0;

  const destinations = useMemo(
    () => [
      {
        label: t('pages.hosts.cleanIp.existingHost'),
        options: hosts.map((h) => ({ value: `g:${h.groupId}`, label: h.remark })),
      },
      {
        label: t('pages.hosts.cleanIp.newHostOnInbound'),
        options: inboundOptions.map((ib) => ({
          value: `i:${ib.id}`,
          label: ib.remark || ib.tag || `#${ib.id}`,
        })),
      },
    ],
    [hosts, inboundOptions, t],
  );

  const onApply = async () => {
    if (!picked || !destination) return;
    const isGroup = destination.startsWith('g:');
    const target = destination.slice(2);
    const msg = await apply.mutateAsync({
      ip: picked.ip,
      mode,
      ...(isGroup ? { groupId: target } : { inboundId: Number(target) }),
    });
    if (msg?.success) setPicked(null);
  };

  const reasonLabel = (reason: string) =>
    KNOWN_REASONS.includes(reason) ? t(`pages.hosts.cleanIp.reasons.${reason}`) : reason;

  const columns: ColumnsType<CleanIPResult> = [
    { title: t('pages.hosts.cleanIp.ip'), dataIndex: 'ip', key: 'ip' },
    {
      title: t('pages.hosts.cleanIp.latency'),
      key: 'latency',
      render: (_, r) => (r.ok ? `${r.latencyMs} ms` : '-'),
    },
    {
      title: t('pages.hosts.cleanIp.loss'),
      dataIndex: 'lossPct',
      key: 'loss',
      render: (v: number) => `${v}%`,
    },
    {
      title: t('pages.hosts.cleanIp.status'),
      key: 'status',
      render: (_, r) =>
        r.ok ? (
          <Tag color="green">{t('pages.hosts.cleanIp.reachable')}</Tag>
        ) : (
          <Tag color="red">{reasonLabel(r.reason)}</Tag>
        ),
    },
    {
      title: '',
      key: 'apply',
      render: (_, r) => (
        <Button size="small" disabled={!r.ok} onClick={() => setPicked(r)}>
          {t('pages.hosts.cleanIp.apply')}
        </Button>
      ),
    },
  ];

  return (
    <>
      <Modal
        open={open}
        title={t('pages.hosts.cleanIp.title')}
        onCancel={onClose}
        footer={null}
        width={720}
      >
        <Form layout="vertical">
          <Form.Item
            label={t('pages.hosts.cleanIp.targets')}
            extra={t('pages.hosts.cleanIp.targetsHint')}
          >
            <Input.TextArea
              rows={3}
              value={targets}
              placeholder="104.16.0.0/24, 172.64.32.1"
              onChange={(e) => setTargets(e.target.value)}
            />
          </Form.Item>
          <Form.Item label={t('pages.hosts.cleanIp.sni')} extra={t('pages.hosts.cleanIp.sniHint')}>
            <Input value={sni} onChange={(e) => setSni(e.target.value)} />
          </Form.Item>
          <Form.Item label={t('pages.hosts.cleanIp.port')}>
            <InputNumber min={1} max={65535} value={port} onChange={(v) => setPort(v ?? 443)} />
          </Form.Item>
          <Form.Item label={t('pages.hosts.cleanIp.samples')}>
            <InputNumber min={1} max={5} value={samples} onChange={(v) => setSamples(v ?? 3)} />
          </Form.Item>
          <Button
            type="primary"
            loading={running || start.isPending}
            disabled={targets.trim() === ''}
            onClick={() => start.mutate({ targets, port, sni, samples })}
          >
            {t('pages.hosts.cleanIp.start')}
          </Button>
        </Form>

        {running && <Progress percent={percent} style={{ marginTop: 16 }} />}
        {status?.error ? (
          <Alert type="error" message={status.error} style={{ marginTop: 16 }} />
        ) : null}

        <Table<CleanIPResult>
          rowKey={(r) => `${r.ip}:${r.port}`}
          size="small"
          style={{ marginTop: 16 }}
          columns={columns}
          dataSource={results}
          pagination={{ pageSize: 10, hideOnSinglePage: true }}
          locale={{ emptyText: t('noData') }}
        />
      </Modal>

      <Modal
        open={picked !== null}
        title={t('pages.hosts.cleanIp.applyTitle', { ip: picked?.ip ?? '' })}
        okText={t('pages.hosts.cleanIp.apply')}
        okButtonProps={{ disabled: !destination, loading: apply.isPending }}
        onOk={onApply}
        onCancel={() => setPicked(null)}
      >
        <Form layout="vertical">
          <Form.Item label={t('pages.hosts.cleanIp.destination')}>
            <Select
              showSearch
              optionFilterProp="label"
              value={destination}
              options={destinations}
              onChange={setDestination}
              style={{ width: '100%' }}
            />
          </Form.Item>
          {destination?.startsWith('g:') && (
            <Radio.Group value={mode} onChange={(e) => setMode(e.target.value)}>
              <Radio value="replace">{t('pages.hosts.cleanIp.modeReplace')}</Radio>
              <Radio value="add">{t('pages.hosts.cleanIp.modeAdd')}</Radio>
            </Radio.Group>
          )}
        </Form>
      </Modal>
    </>
  );
}
