import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Descriptions, Modal, Spin } from 'antd';

import { HttpUtil } from '@/utils';
import { parseMsg } from '@/utils/zodValidate';
import { NodeSpeedResultSchema, type NodeSpeedResult } from '@/schemas/node';
import type { NodeRecord } from '@/api/queries/useNodesQuery';

interface NodeSpeedModalProps {
  node: NodeRecord | null;
  onClose: () => void;
}

export default function NodeSpeedModal({ node, onClose }: NodeSpeedModalProps) {
  const { t } = useTranslation();
  return (
    <Modal
      open={!!node}
      destroyOnHidden
      title={`${t('pages.nodes.speedTest.title')} — ${node?.name ?? ''}`}
      footer={null}
      onCancel={onClose}
    >
      {node && <SpeedBody node={node} />}
    </Modal>
  );
}

// Mounted per open, so each run starts from empty state without a reset effect.
function SpeedBody({ node }: { node: NodeRecord }) {
  const { t } = useTranslation();
  const [result, setResult] = useState<NodeSpeedResult | null>(null);
  const [error, setError] = useState('');

  useEffect(() => {
    let cancelled = false;
    HttpUtil.post(`/panel/api/nodes/speedtest/${node.id}`)
      .then((raw) => {
        if (cancelled) return;
        const msg = parseMsg(raw, NodeSpeedResultSchema, 'nodes/speedtest');
        if (msg?.success && msg.obj) setResult(msg.obj);
        else setError(msg?.msg || t('pages.nodes.speedTest.failed'));
      })
      .catch(() => {
        if (!cancelled) setError(t('pages.nodes.speedTest.failed'));
      });
    return () => {
      cancelled = true;
    };
  }, [node, t]);

  const ms = (v: number) => `${v.toFixed(1)} ms`;
  const mbps = (v: number) => `${v.toFixed(1)} Mbit/s`;

  return (
    <>
      {error && <Alert type="error" showIcon title={error} />}
      {!error && !result && (
        <Spin description={t('pages.nodes.speedTest.running')}>
          <div style={{ minHeight: 96 }} />
        </Spin>
      )}
      {result && (
        <>
          <Descriptions column={1} size="small" bordered>
            <Descriptions.Item label={t('pages.nodes.speedTest.latency')}>
              {`${ms(result.latencyMinMs)} / ${ms(result.latencyAvgMs)} / ${ms(result.latencyMaxMs)}`}
            </Descriptions.Item>
            <Descriptions.Item label={t('pages.nodes.speedTest.jitter')}>
              {ms(result.jitterMs)}
            </Descriptions.Item>
            <Descriptions.Item label={t('pages.nodes.speedTest.lost')}>
              {`${result.lost} / ${result.samples + result.lost}`}
            </Descriptions.Item>
            <Descriptions.Item label={t('pages.nodes.speedTest.download')}>
              {result.throughputError ? '—' : mbps(result.downloadMbps)}
            </Descriptions.Item>
            <Descriptions.Item label={t('pages.nodes.speedTest.upload')}>
              {result.throughputError ? '—' : mbps(result.uploadMbps)}
            </Descriptions.Item>
          </Descriptions>
          {result.throughputError && (
            <Alert
              style={{ marginTop: 12 }}
              type="warning"
              showIcon
              title={t('pages.nodes.speedTest.throughputUnavailable')}
              description={result.throughputError}
            />
          )}
        </>
      )}
    </>
  );
}
