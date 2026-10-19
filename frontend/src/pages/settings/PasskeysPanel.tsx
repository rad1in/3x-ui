import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Empty, Form, Input, List, Modal, Space, Typography, message } from 'antd';
import { DeleteOutlined, PlusOutlined } from '@ant-design/icons';

import { HttpUtil, IntlUtil } from '@/utils';
import { createPasskey, passkeysSupported, type CreationOptionsJSON } from '@/lib/passkey';

interface PasskeyRow {
  id: number;
  name: string;
  createdAt: number;
  lastUsedAt: number;
}

const JSON_HEADERS = { headers: { 'Content-Type': 'application/json' } } as const;

// The signed-in admin's own passkeys; any role may manage them.
export default function PasskeysPanel() {
  const { t } = useTranslation();
  const [messageApi, messageContextHolder] = message.useMessage();
  const [modal, modalContextHolder] = Modal.useModal();
  const [rows, setRows] = useState<PasskeyRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [addOpen, setAddOpen] = useState(false);
  const [name, setName] = useState('');
  const [password, setPassword] = useState('');
  const [adding, setAdding] = useState(false);

  const load = useCallback(async () => {
    try {
      const msg = await HttpUtil.get('/panel/api/passkeys/list', undefined, { silent: true });
      if (msg?.success) setRows(Array.isArray(msg.obj) ? (msg.obj as PasskeyRow[]) : []);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  async function addPasskey() {
    setAdding(true);
    try {
      const begin = await HttpUtil.post(
        '/panel/api/passkeys/register/begin',
        { password },
        JSON_HEADERS,
      );
      const obj = begin?.obj as { options?: CreationOptionsJSON; challengeId?: string } | null;
      if (!begin?.success || !obj?.options) return;
      let credential;
      try {
        credential = await createPasskey(obj.options);
      } catch {
        messageApi.error(t('pages.login.toasts.passkeyFailed'));
        return;
      }
      const done = await HttpUtil.post(
        '/panel/api/passkeys/register/finish',
        { challengeId: obj.challengeId, name: name.trim(), credential },
        JSON_HEADERS,
      );
      if (done?.success) {
        setAddOpen(false);
        setName('');
        setPassword('');
        await load();
      }
    } finally {
      setAdding(false);
    }
  }

  function confirmDelete(row: PasskeyRow) {
    modal.confirm({
      title: t('pages.settings.security.passkeys.deleteConfirm'),
      content: row.name,
      okText: t('pages.settings.security.passkeys.delete'),
      cancelText: t('cancel'),
      okType: 'danger',
      onOk: async () => {
        const msg = await HttpUtil.post(`/panel/api/passkeys/del/${row.id}`);
        if (msg?.success) await load();
      },
    });
  }

  if (!passkeysSupported()) {
    return <Empty description={t('pages.settings.security.passkeys.unsupported')} />;
  }

  return (
    <>
      {messageContextHolder}
      {modalContextHolder}
      <Space direction="vertical" size="middle" style={{ width: '100%' }}>
        <Typography.Paragraph type="secondary">
          {t('pages.settings.security.passkeys.intro')}
        </Typography.Paragraph>
        <List
          loading={loading}
          bordered
          dataSource={rows}
          locale={{ emptyText: t('pages.settings.security.passkeys.empty') }}
          renderItem={(row) => (
            <List.Item
              actions={[
                <Button
                  key="del"
                  danger
                  type="text"
                  icon={<DeleteOutlined />}
                  aria-label={t('pages.settings.security.passkeys.delete')}
                  onClick={() => confirmDelete(row)}
                />,
              ]}
            >
              <List.Item.Meta
                title={row.name}
                description={`${t('pages.settings.security.passkeys.created')}: ${IntlUtil.formatDate(row.createdAt)} · ${t('pages.settings.security.passkeys.lastUsed')}: ${row.lastUsedAt ? IntlUtil.formatDate(row.lastUsedAt) : t('pages.settings.security.passkeys.never')}`}
              />
            </List.Item>
          )}
        />
        <Button type="primary" icon={<PlusOutlined />} onClick={() => setAddOpen(true)}>
          {t('pages.settings.security.passkeys.add')}
        </Button>
      </Space>
      <Modal
        open={addOpen}
        title={t('pages.settings.security.passkeys.add')}
        confirmLoading={adding}
        okText={t('confirm')}
        cancelText={t('cancel')}
        okButtonProps={{ disabled: password === '' }}
        onOk={addPasskey}
        onCancel={() => setAddOpen(false)}
      >
        <Form layout="vertical">
          <Form.Item label={t('pages.settings.security.passkeys.name')}>
            <Input
              value={name}
              maxLength={64}
              placeholder={t('pages.settings.security.passkeys.namePlaceholder')}
              onChange={(e) => setName(e.target.value)}
            />
          </Form.Item>
          <Form.Item label={t('pages.settings.security.passkeys.passwordTitle')} required>
            <Input.Password
              value={password}
              autoComplete="current-password"
              onChange={(e) => setPassword(e.target.value)}
            />
          </Form.Item>
        </Form>
      </Modal>
    </>
  );
}
