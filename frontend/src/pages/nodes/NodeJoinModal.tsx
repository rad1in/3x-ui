import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Input, Modal, Space, Typography, message } from 'antd';
import { CopyOutlined } from '@ant-design/icons';

import { HttpUtil } from '@/utils';
import { NodeJoinCommandSchema, type NodeJoinCommand } from '@/generated/zod';

interface NodeJoinModalProps {
  open: boolean;
  onClose: () => void;
}

export default function NodeJoinModal({ open, onClose }: NodeJoinModalProps) {
  const { t } = useTranslation();
  const [messageApi, messageContextHolder] = message.useMessage();
  const [name, setName] = useState('');
  const [busy, setBusy] = useState(false);
  const [issued, setIssued] = useState<NodeJoinCommand | null>(null);

  const issue = async () => {
    setBusy(true);
    try {
      const msg = await HttpUtil.post(
        '/panel/api/nodes/joinCommand',
        { name: name.trim() },
        { headers: { 'Content-Type': 'application/json' } },
      );
      const parsed = NodeJoinCommandSchema.safeParse(msg?.obj);
      if (msg?.success && parsed.success) setIssued(parsed.data);
    } finally {
      setBusy(false);
    }
  };

  const copy = async () => {
    if (!issued) return;
    try {
      await navigator.clipboard.writeText(issued.command);
      messageApi.success(t('pages.nodes.join.copied'));
    } catch {
      messageApi.error(t('somethingWentWrong'));
    }
  };

  const close = () => {
    setIssued(null);
    setName('');
    onClose();
  };

  return (
    <Modal
      open={open}
      title={t('pages.nodes.join.title')}
      onCancel={close}
      footer={null}
      width={640}
    >
      {messageContextHolder}
      <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
        <Typography.Paragraph>{t('pages.nodes.join.intro')}</Typography.Paragraph>
        <Space.Compact style={{ width: '100%' }}>
          <Input
            value={name}
            placeholder={t('pages.nodes.join.namePlaceholder')}
            onChange={(e) => setName(e.target.value)}
          />
          <Button type="primary" loading={busy} onClick={issue}>
            {t('pages.nodes.join.generate')}
          </Button>
        </Space.Compact>
        {issued && (
          <>
            <Input.TextArea
              readOnly
              rows={4}
              value={issued.command}
              style={{ direction: 'ltr', fontFamily: 'monospace' }}
            />
            <Button icon={<CopyOutlined />} onClick={copy}>
              {t('copy')}
            </Button>
            <Alert
              type="warning"
              showIcon
              message={t('pages.nodes.join.expires', {
                time: new Date(issued.expiresAt).toLocaleTimeString(),
              })}
              description={t('pages.nodes.join.note')}
            />
          </>
        )}
      </Space>
    </Modal>
  );
}
