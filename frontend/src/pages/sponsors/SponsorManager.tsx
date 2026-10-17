import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Card, Form, Input, Popconfirm, Space, Table, Tag } from 'antd';
import type { TableColumnsType } from 'antd';
import { DeleteOutlined, EditOutlined, PlusOutlined } from '@ant-design/icons';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { keys } from '@/api/queryKeys';
import { SponsorAdminViewSchema, type SponsorConfig, type SponsorRecord } from '@/generated/zod';
import { useDatepicker } from '@/hooks/useDatepicker';
import type { SponsorFormValues } from '@/schemas/sponsor';
import { HttpUtil, IntlUtil } from '@/utils';
import { parseMsg } from '@/utils/zodValidate';
import SponsorFormModal, { SLOT_LABEL_KEYS } from './SponsorFormModal';

const JSON_HEADERS = { headers: { 'Content-Type': 'application/json' } } as const;

async function fetchAdmin() {
  const msg = await HttpUtil.get('/panel/api/sponsors/list', undefined, { silent: true });
  if (!msg?.success) throw new Error(msg?.msg || 'Failed to load sponsors');
  return parseMsg(msg, SponsorAdminViewSchema, 'sponsors/list').obj;
}

export default function SponsorManager() {
  const { t } = useTranslation();
  const { datepicker } = useDatepicker();
  const queryClient = useQueryClient();
  const { data, isLoading } = useQuery({ queryKey: keys.sponsorAdmin(), queryFn: fetchAdmin });

  const [draft, setDraft] = useState<SponsorConfig | null>(null);
  const saved = data?.config ?? { feedUrl: '', contact: '' };
  const config = draft ?? saved;

  const [editing, setEditing] = useState<SponsorRecord | null>(null);
  const [modalOpen, setModalOpen] = useState(false);

  const refresh = () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: keys.sponsorAdmin() }),
      queryClient.invalidateQueries({ queryKey: keys.sponsors() }),
    ]);

  const save = useMutation({
    mutationFn: (values: SponsorFormValues) =>
      HttpUtil.post('/panel/api/sponsors/save', { ...values, id: editing?.id ?? 0 }, JSON_HEADERS),
    onSuccess: async (msg) => {
      if (!msg?.success) return;
      setModalOpen(false);
      await refresh();
    },
  });
  const remove = useMutation({
    mutationFn: (id: number) => HttpUtil.post(`/panel/api/sponsors/del/${id}`),
    onSuccess: refresh,
  });
  const saveConfig = useMutation({
    mutationFn: (c: SponsorConfig) => HttpUtil.post('/panel/api/sponsors/config', c, JSON_HEADERS),
    onSuccess: async (msg) => {
      if (!msg?.success) return;
      await refresh();
      setDraft(null);
    },
  });

  const formatWindow = (from: number, until: number) => {
    if (!from && !until) return t('pages.sponsors.admin.always');
    const start = from ? IntlUtil.formatDate(from, datepicker) : '…';
    const end = until ? IntlUtil.formatDate(until, datepicker) : '…';
    return `${start} → ${end}`;
  };

  const columns: TableColumnsType<SponsorRecord> = [
    {
      title: t('pages.sponsors.admin.name'),
      dataIndex: 'name',
      render: (name: string, r) => (
        <Space>
          {r.logo ? (
            <img className="sponsor-admin-logo" src={r.logo} alt="" />
          ) : (
            <span className="sponsor-admin-logo sponsor-admin-logo-fallback">
              {name.slice(0, 1).toUpperCase()}
            </span>
          )}
          <span>
            <div className="sponsor-admin-name">{name}</div>
            {r.title && (
              <div className="sponsor-admin-title" dir="auto">
                {r.title}
              </div>
            )}
          </span>
        </Space>
      ),
    },
    {
      title: t('pages.sponsors.admin.slots'),
      dataIndex: 'slots',
      render: (slots: string[] | null) => (
        <Space size={4} wrap>
          {(slots ?? []).map((s) => (
            <Tag key={s}>
              {s in SLOT_LABEL_KEYS ? t(SLOT_LABEL_KEYS[s as keyof typeof SLOT_LABEL_KEYS]) : s}
            </Tag>
          ))}
        </Space>
      ),
    },
    {
      title: t('pages.sponsors.admin.window'),
      render: (_, r) => (
        <span className="sponsor-admin-window">{formatWindow(r.from, r.until)}</span>
      ),
    },
    {
      title: t('status'),
      dataIndex: 'enable',
      width: 100,
      render: (on: boolean) => (
        <Tag color={on ? 'success' : 'default'}>{on ? t('enabled') : t('disabled')}</Tag>
      ),
    },
    {
      width: 100,
      render: (_, r) => (
        <Space>
          <Button
            size="small"
            icon={<EditOutlined />}
            aria-label={t('edit')}
            onClick={() => {
              setEditing(r);
              setModalOpen(true);
            }}
          />
          <Popconfirm
            title={t('pages.sponsors.admin.deleteConfirm')}
            onConfirm={() => remove.mutate(r.id)}
          >
            <Button size="small" danger icon={<DeleteOutlined />} aria-label={t('delete')} />
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <div className="sponsor-admin">
      <Card
        size="small"
        title={t('pages.sponsors.admin.title')}
        extra={
          <Button
            type="primary"
            icon={<PlusOutlined />}
            onClick={() => {
              setEditing(null);
              setModalOpen(true);
            }}
          >
            {t('pages.sponsors.admin.add')}
          </Button>
        }
      >
        {saved.feedUrl && (
          <p className="sponsor-admin-note">{t('pages.sponsors.admin.feedActive')}</p>
        )}
        <Table<SponsorRecord>
          rowKey="id"
          size="small"
          loading={isLoading}
          columns={columns}
          dataSource={data?.records ?? []}
          pagination={false}
          scroll={{ x: 700 }}
        />
      </Card>

      <Card size="small" title={t('pages.sponsors.admin.setup')}>
        <Form layout="vertical">
          <Form.Item
            label={t('pages.sponsors.admin.contact')}
            extra={t('pages.sponsors.admin.contactHint')}
          >
            <Input
              value={config.contact}
              placeholder="https://t.me/…"
              onChange={(e) => setDraft({ ...config, contact: e.target.value })}
            />
          </Form.Item>
          <Form.Item
            label={t('pages.sponsors.admin.feedUrl')}
            extra={t('pages.sponsors.admin.feedUrlHint')}
          >
            <Input
              value={config.feedUrl}
              placeholder="https://…/sponsors/feed/sponsors.json"
              onChange={(e) => setDraft({ ...config, feedUrl: e.target.value })}
            />
          </Form.Item>
          <p className="sponsor-admin-note">
            {t('pages.sponsors.admin.ownFeed')}{' '}
            <code>{`${window.location.origin}${window.X_UI_BASE_PATH || '/'}sponsors/feed/sponsors.json`}</code>
          </p>
          <Button loading={saveConfig.isPending} onClick={() => saveConfig.mutate(config)}>
            {t('save')}
          </Button>
        </Form>
      </Card>

      <SponsorFormModal
        open={modalOpen}
        record={editing}
        saving={save.isPending}
        onClose={() => setModalOpen(false)}
        onSubmit={(values) => save.mutate(values)}
      />
    </div>
  );
}
