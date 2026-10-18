import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Checkbox,
  Divider,
  Empty,
  Input,
  InputNumber,
  List,
  Modal,
  Popconfirm,
  Select,
  Space,
  message,
} from 'antd';
import { DeleteOutlined, EditOutlined, PlusOutlined } from '@ant-design/icons';

import { formatInboundLabel } from '@/lib/inbounds/label';
import { TLS_FLOW_CONTROL } from '@/schemas/primitives';
import { ClientTemplateSchema, type ClientTemplate } from '@/schemas/clientTemplate';
import { useClientTemplates } from '@/api/queries/useClientTemplates';
import type { InboundOption } from '@/hooks/useClients';

const BYTES_PER_GB = 1024 ** 3;
const FLOW_OPTIONS = Object.values(TLS_FLOW_CONTROL);

const blankTemplate = (): ClientTemplate => ({
  name: '',
  totalGB: 0,
  expiryDays: 0,
  delayedStart: false,
  limitIp: 0,
  flow: '',
  group: '',
  comment: '',
  reset: 0,
  trafficReset: 'never',
  trafficResetDay: 1,
  inboundIds: [],
});

interface ClientTemplatesModalProps {
  open: boolean;
  inbounds: InboundOption[];
  groups?: string[];
  onOpenChange: (open: boolean) => void;
}

export default function ClientTemplatesModal({
  open,
  inbounds,
  groups = [],
  onOpenChange,
}: ClientTemplatesModalProps) {
  const { t } = useTranslation();
  const [messageApi, messageContextHolder] = message.useMessage();
  const { templates, save, remove, apply, applying } = useClientTemplates(open);

  const [editing, setEditing] = useState<ClientTemplate | null>(null);
  const [selectedId, setSelectedId] = useState<number | undefined>();
  const [emailsText, setEmailsText] = useState('');

  const inboundOptions = inbounds.map((ib) => ({
    value: ib.id,
    label: formatInboundLabel(ib.tag, ib.remark) || String(ib.id),
  }));

  const patch = (next: Partial<ClientTemplate>) =>
    setEditing((cur) => (cur ? { ...cur, ...next } : cur));

  async function onSave() {
    if (!editing) return;
    const parsed = ClientTemplateSchema.safeParse(editing);
    if (!parsed.success) {
      messageApi.error(t('somethingWentWrong'));
      return;
    }
    const msg = await save(parsed.data);
    if (msg?.success) {
      messageApi.success(t('pages.clients.templates.saved'));
      setEditing(null);
    }
  }

  async function onApply() {
    const emails = emailsText
      .split('\n')
      .map((line) => line.trim())
      .filter(Boolean);
    if (selectedId == null || emails.length === 0) return;
    const msg = await apply({ templateId: selectedId, emails });
    const result = msg?.obj;
    if (!msg?.success || !result) return;
    const failed = Object.keys(result.failed);
    messageApi.open({
      type: failed.length > 0 ? 'warning' : 'success',
      content: t('pages.clients.templates.result', {
        created: result.created.length,
        failed: failed.length,
      }),
    });
    setEmailsText(failed.join('\n'));
  }

  return (
    <Modal
      open={open}
      title={t('pages.clients.templates.title')}
      footer={null}
      width={640}
      onCancel={() => onOpenChange(false)}
      destroyOnHidden
    >
      {messageContextHolder}
      {editing ? (
        <Space direction="vertical" size={10} style={{ width: '100%' }}>
          <Input
            value={editing.name}
            placeholder={t('pages.clients.templates.name')}
            onChange={(e) => patch({ name: e.target.value })}
          />
          <InputNumber
            min={0}
            style={{ width: '100%' }}
            addonBefore={t('pages.clients.templates.quotaGb')}
            value={editing.totalGB / BYTES_PER_GB}
            onChange={(v) => patch({ totalGB: Math.round((v ?? 0) * BYTES_PER_GB) })}
          />
          <InputNumber
            min={0}
            precision={0}
            style={{ width: '100%' }}
            addonBefore={t('pages.clients.templates.durationDays')}
            value={editing.expiryDays}
            onChange={(v) => patch({ expiryDays: v ?? 0 })}
          />
          <Checkbox
            checked={editing.delayedStart}
            onChange={(e) => patch({ delayedStart: e.target.checked })}
          >
            {t('pages.clients.templates.delayedStart')}
          </Checkbox>
          <InputNumber
            min={0}
            precision={0}
            style={{ width: '100%' }}
            addonBefore={t('pages.clients.limitIp')}
            value={editing.limitIp}
            onChange={(v) => patch({ limitIp: v ?? 0 })}
          />
          <Select
            allowClear
            style={{ width: '100%' }}
            placeholder={t('pages.clients.flow')}
            value={editing.flow || undefined}
            options={FLOW_OPTIONS.map((f) => ({ value: f, label: f }))}
            onChange={(v) => patch({ flow: v ?? '' })}
          />
          <Select
            mode="tags"
            maxCount={1}
            style={{ width: '100%' }}
            placeholder={t('pages.clients.group')}
            value={editing.group ? [editing.group] : []}
            options={groups.map((g) => ({ value: g, label: g }))}
            onChange={(v: string[]) => patch({ group: v[v.length - 1] ?? '' })}
          />
          <Input
            value={editing.comment}
            placeholder={t('pages.clients.comment')}
            onChange={(e) => patch({ comment: e.target.value })}
          />
          <Select
            mode="multiple"
            style={{ width: '100%' }}
            placeholder={t('pages.clients.attachedInbounds')}
            value={editing.inboundIds}
            options={inboundOptions}
            onChange={(v: number[]) => patch({ inboundIds: v })}
          />
          <Space>
            <Button type="primary" onClick={onSave}>
              {t('save')}
            </Button>
            <Button onClick={() => setEditing(null)}>{t('cancel')}</Button>
          </Space>
        </Space>
      ) : (
        <>
          <Button icon={<PlusOutlined />} onClick={() => setEditing(blankTemplate())}>
            {t('pages.clients.templates.newTemplate')}
          </Button>
          <List
            size="small"
            locale={{ emptyText: <Empty description={t('pages.clients.templates.noTemplates')} /> }}
            dataSource={templates}
            renderItem={(tpl) => (
              <List.Item
                actions={[
                  <Button
                    key="edit"
                    type="text"
                    icon={<EditOutlined />}
                    onClick={() => setEditing(tpl)}
                  />,
                  <Popconfirm
                    key="del"
                    title={t('pages.clients.templates.confirmDelete')}
                    onConfirm={() => tpl.id != null && remove(tpl.id)}
                  >
                    <Button type="text" danger icon={<DeleteOutlined />} />
                  </Popconfirm>,
                ]}
              >
                {tpl.name}
              </List.Item>
            )}
          />
          <Divider />
          <Space direction="vertical" size={10} style={{ width: '100%' }}>
            <Select
              style={{ width: '100%' }}
              placeholder={t('pages.clients.templates.selectTemplate')}
              value={selectedId}
              options={templates.map((tpl) => ({ value: tpl.id, label: tpl.name }))}
              onChange={setSelectedId}
            />
            <Input.TextArea
              rows={4}
              value={emailsText}
              placeholder={t('pages.clients.templates.emails')}
              onChange={(e) => setEmailsText(e.target.value)}
            />
            <Button
              type="primary"
              loading={applying}
              disabled={selectedId == null || emailsText.trim() === ''}
              onClick={onApply}
            >
              {t('pages.clients.templates.createClients')}
            </Button>
          </Space>
        </>
      )}
    </Modal>
  );
}
