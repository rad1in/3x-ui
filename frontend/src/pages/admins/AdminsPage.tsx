import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Card, ConfigProvider, Layout, Popconfirm, Space, Table, Tag, message } from 'antd';
import type { TableColumnsType } from 'antd';
import { DeleteOutlined, EditOutlined, PlusOutlined, UserSwitchOutlined } from '@ant-design/icons';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { z } from 'zod';

import { keys } from '@/api/queryKeys';
import { useCurrentAdmin } from '@/api/queries/useCurrentAdmin';
import { useInboundOptions } from '@/api/queries/useInboundOptions';
import { AdminViewSchema, type AdminView } from '@/generated/zod';
import { usePageTitle } from '@/hooks/usePageTitle';
import { useTheme } from '@/hooks/useTheme';
import AppSidebar from '@/layouts/AppSidebar';
import { GroupSummaryListSchema } from '@/schemas/client';
import type { AdminFormValues } from '@/schemas/admin';
import { HttpUtil } from '@/utils';
import { setMessageInstance } from '@/utils/messageBus';
import { parseMsg } from '@/utils/zodValidate';
import AdminFormModal from './AdminFormModal';
import './AdminsPage.css';

const JSON_HEADERS = { headers: { 'Content-Type': 'application/json' } } as const;

const ROLE_COLORS: Record<string, string> = {
  owner: 'gold',
  admin: 'blue',
  operator: 'green',
  viewer: 'default',
};

async function fetchAdmins(): Promise<AdminView[]> {
  const msg = await HttpUtil.get('/panel/api/admins/list', undefined, { silent: true });
  if (!msg?.success) throw new Error(msg?.msg || 'Failed to load admins');
  return parseMsg(msg, z.array(AdminViewSchema), 'admins/list').obj ?? [];
}

async function fetchGroupNames(): Promise<string[]> {
  const msg = await HttpUtil.get('/panel/api/clients/groups', undefined, { silent: true });
  if (!msg?.success) return [];
  return (parseMsg(msg, GroupSummaryListSchema, 'clients/groups').obj ?? []).map((g) => g.name);
}

export default function AdminsPage() {
  const { t } = useTranslation();
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  const [messageApi, messageContextHolder] = message.useMessage();
  const queryClient = useQueryClient();
  usePageTitle();
  useEffect(() => {
    setMessageInstance(messageApi);
  }, [messageApi]);

  const me = useCurrentAdmin().data;
  const { data: admins = [], isLoading } = useQuery({
    queryKey: keys.admins.list(),
    queryFn: fetchAdmins,
  });
  const { data: inbounds = [] } = useInboundOptions();
  const { data: groupNames = [] } = useQuery({
    queryKey: keys.clients.groups(),
    queryFn: fetchGroupNames,
  });
  const inboundOptions = useMemo(
    () => inbounds.map((ib) => ({ value: ib.id, label: ib.remark || `#${ib.id}` })),
    [inbounds],
  );
  const inboundLabel = useMemo(
    () => new Map(inboundOptions.map((o) => [o.value, o.label])),
    [inboundOptions],
  );

  const [editing, setEditing] = useState<AdminView | null>(null);
  const [modalOpen, setModalOpen] = useState(false);
  const refresh = () => queryClient.invalidateQueries({ queryKey: keys.admins.list() });

  const save = useMutation({
    mutationFn: (values: AdminFormValues) =>
      editing
        ? HttpUtil.post(`/panel/api/admins/update/${editing.id}`, values, JSON_HEADERS)
        : HttpUtil.post('/panel/api/admins/create', values, JSON_HEADERS),
    onSuccess: async (msg) => {
      if (!msg?.success) return;
      setModalOpen(false);
      await refresh();
    },
  });
  const remove = useMutation({
    mutationFn: (id: number) => HttpUtil.post(`/panel/api/admins/del/${id}`),
    onSuccess: refresh,
  });

  const openEditor = (admin: AdminView | null) => {
    setEditing(admin);
    setModalOpen(true);
  };

  const columns: TableColumnsType<AdminView> = [
    {
      title: t('username'),
      dataIndex: 'username',
      render: (name: string, a) => (
        <span className="admins-name" dir="auto">
          {name}
          {a.id === me?.id && <Tag className="admins-you">{t('pages.admins.you')}</Tag>}
        </span>
      ),
    },
    {
      title: t('pages.admins.role'),
      dataIndex: 'role',
      width: 140,
      render: (role: string) => (
        <Tag color={ROLE_COLORS[role]}>{t(`pages.admins.roles.${role}`)}</Tag>
      ),
    },
    {
      title: t('pages.admins.scope'),
      render: (_, a) =>
        a.role !== 'operator' ? (
          <span className="admins-sub">{t('pages.admins.scopeAll')}</span>
        ) : (
          <Space size={[4, 4]} wrap>
            {(a.inboundIds ?? []).map((id) => (
              <Tag key={`i${id}`}>{inboundLabel.get(id) ?? `#${id}`}</Tag>
            ))}
            {(a.groups ?? []).map((g) => (
              <Tag key={`g${g}`} color="purple">
                {g}
              </Tag>
            ))}
            {(a.inboundIds ?? []).length + (a.groups ?? []).length === 0 && (
              <span className="admins-sub">{t('pages.admins.scopeNone')}</span>
            )}
          </Space>
        ),
    },
    {
      width: 90,
      render: (_, a) => (
        <Space size={4}>
          <Button
            size="small"
            icon={<EditOutlined />}
            aria-label={t('edit')}
            onClick={() => openEditor(a)}
          />
          <Popconfirm
            title={t('pages.admins.deleteConfirm')}
            okButtonProps={{ danger: true }}
            disabled={a.id === me?.id}
            onConfirm={() => remove.mutate(a.id)}
          >
            <Button
              size="small"
              danger
              disabled={a.id === me?.id}
              icon={<DeleteOutlined />}
              aria-label={t('delete')}
            />
          </Popconfirm>
        </Space>
      ),
    },
  ];

  const pageClass = `admins-page ${isDark ? 'is-dark' : ''} ${isUltra ? 'is-ultra' : ''}`.trim();

  return (
    <ConfigProvider theme={antdThemeConfig}>
      {messageContextHolder}
      <Layout className={pageClass}>
        <AppSidebar />
        <Layout className="content-shell">
          <Layout.Content className="content-area">
            <Card
              size="small"
              title={
                <span className="admins-title">
                  <UserSwitchOutlined /> {t('pages.admins.title')}
                </span>
              }
              extra={
                <Button type="primary" icon={<PlusOutlined />} onClick={() => openEditor(null)}>
                  {t('pages.admins.add')}
                </Button>
              }
            >
              <p className="admins-hint" dir="auto">
                {t('pages.admins.hint')}
              </p>
              <Table<AdminView>
                rowKey="id"
                size="small"
                loading={isLoading}
                columns={columns}
                dataSource={admins}
                pagination={false}
                scroll={{ x: 640 }}
              />
            </Card>
            <AdminFormModal
              open={modalOpen}
              admin={editing}
              isSelf={editing !== null && editing.id === me?.id}
              saving={save.isPending}
              inboundOptions={inboundOptions}
              groupOptions={groupNames}
              onClose={() => setModalOpen(false)}
              onSubmit={(values) => save.mutate(values)}
            />
          </Layout.Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  );
}
