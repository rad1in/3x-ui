import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Form, Input, InputNumber, Modal, Space, Switch, message } from 'antd';

import { HttpUtil } from '@/utils';
import { onNumber } from '@/utils/onNumber';
import { InboundRotationSchema, type InboundRotation } from '@/generated/zod';
import type { DBInbound } from '@/models/dbinbound';
import { readStreamHints } from './list/helpers';

interface RotationModalProps {
  dbInbound: DBInbound | null;
  onClose: () => void;
  onRotated: () => void | Promise<void>;
}

export default function RotationModal({ dbInbound, onClose, onRotated }: RotationModalProps) {
  const { t } = useTranslation();
  const [messageApi, messageContextHolder] = message.useMessage();
  const [cfg, setCfg] = useState<InboundRotation | null>(null);
  const [busy, setBusy] = useState(false);
  const id = dbInbound?.id;
  const isReality = dbInbound ? readStreamHints(dbInbound.streamSettings).isReality : false;

  useEffect(() => {
    if (id === undefined) return;
    let live = true;
    void HttpUtil.get(`/panel/api/inbounds/${id}/rotation`, undefined, { silent: true }).then(
      (msg) => {
        if (!live || !msg?.success) return;
        const parsed = InboundRotationSchema.safeParse(msg.obj);
        if (parsed.success) setCfg(parsed.data);
      },
    );
    return () => {
      live = false;
      setCfg(null);
    };
  }, [id]);

  if (!dbInbound) return null;
  const patch = (p: Partial<InboundRotation>) => setCfg((c) => (c ? { ...c, ...p } : c));
  const url = `/panel/api/inbounds/${dbInbound.id}`;
  const headers = { headers: { 'Content-Type': 'application/json' } };

  const save = async () => {
    if (!cfg) return false;
    const msg = await HttpUtil.post(`${url}/rotation`, cfg, headers);
    if (msg?.success) {
      const parsed = InboundRotationSchema.safeParse(msg.obj);
      if (parsed.success) setCfg(parsed.data);
    }
    return !!msg?.success;
  };

  const run = async (rotateNow: boolean) => {
    setBusy(true);
    try {
      if (!(await save())) return;
      if (!rotateNow) {
        messageApi.success(t('pages.inbounds.rotation.saved'));
        return;
      }
      const msg = await HttpUtil.post(`${url}/rotate`);
      if (msg?.success) {
        await onRotated();
        messageApi.success(t('pages.inbounds.rotation.rotated'));
      }
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal
      open
      title={t('pages.inbounds.rotation.title', { name: dbInbound.remark })}
      onCancel={onClose}
      footer={
        <Space>
          <Button onClick={onClose}>{t('close')}</Button>
          <Button loading={busy} disabled={!cfg} onClick={() => run(false)}>
            {t('pages.inbounds.rotation.save')}
          </Button>
          <Button type="primary" loading={busy} disabled={!cfg} onClick={() => run(true)}>
            {t('pages.inbounds.rotation.saveAndRotate')}
          </Button>
        </Space>
      }
    >
      {messageContextHolder}
      <Alert type="info" showIcon message={t('pages.inbounds.rotation.hint')} />
      {cfg && (
        <Form layout="vertical" style={{ marginTop: 16 }}>
          <Form.Item label={t('pages.inbounds.rotation.rotatePort')}>
            <Switch checked={cfg.rotatePort} onChange={(v) => patch({ rotatePort: v })} />
          </Form.Item>
          {cfg.rotatePort && (
            <Space>
              <InputNumber
                min={1}
                max={65535}
                value={cfg.portMin}
                onChange={onNumber((v) => patch({ portMin: v }))}
              />
              <InputNumber
                min={1}
                max={65535}
                value={cfg.portMax}
                onChange={onNumber((v) => patch({ portMax: v }))}
              />
            </Space>
          )}
          {isReality && (
            <Form.Item label={t('pages.inbounds.rotation.rotateSni')}>
              <Switch checked={cfg.rotateSni} onChange={(v) => patch({ rotateSni: v })} />
            </Form.Item>
          )}
          {isReality && cfg.rotateSni && (
            <Form.Item extra={t('pages.inbounds.rotation.sniHint')}>
              <Input.TextArea
                rows={4}
                value={cfg.sniList}
                placeholder={'www.microsoft.com\nwww.apple.com:443'}
                style={{ direction: 'ltr' }}
                onChange={(e) => patch({ sniList: e.target.value })}
              />
            </Form.Item>
          )}
          <Form.Item
            label={t('pages.inbounds.rotation.interval')}
            extra={t('pages.inbounds.rotation.intervalHint')}
          >
            <InputNumber
              min={0}
              max={8760}
              value={cfg.intervalHours}
              onChange={onNumber((v) => patch({ intervalHours: v }))}
            />
          </Form.Item>
          {cfg.lastRotatedAt > 0 && (
            <Alert
              type="success"
              message={t('pages.inbounds.rotation.last', {
                time: new Date(cfg.lastRotatedAt).toLocaleString(),
                port: cfg.lastPort,
              })}
            />
          )}
        </Form>
      )}
    </Modal>
  );
}
