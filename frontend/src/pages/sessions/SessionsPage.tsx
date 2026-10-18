import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Card, ConfigProvider, Layout, Popconfirm, Space, Table, Tag, message } from 'antd';
import type { TableColumnsType } from 'antd';
import { DeleteOutlined, LogoutOutlined, SafetyCertificateOutlined } from '@ant-design/icons';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { z } from 'zod';

import { keys } from '@/api/queryKeys';
import { useCurrentAdmin } from '@/api/queries/useCurrentAdmin';
import { SessionViewSchema, type SessionView } from '@/generated/zod';
import { useDatepicker } from '@/hooks/useDatepicker';
import { usePageTitle } from '@/hooks/usePageTitle';
import { useTheme } from '@/hooks/useTheme';
import AppSidebar from '@/layouts/AppSidebar';
import { HttpUtil, IntlUtil } from '@/utils';
import { setMessageInstance } from '@/utils/messageBus';
import { parseMsg } from '@/utils/zodValidate';
import './SessionsPage.css';

const JSON_HEADERS = { headers: { 'Content-Type': 'application/json' } } as const;
const POLL_MS = 15_000;

async function fetchSessions(): Promise<SessionView[]> {
  const msg = await HttpUtil.get('/panel/api/sessions/list', undefined, { silent: true });
  if (!msg?.success) throw new Error(msg?.msg || 'Failed to load sessions');
  return parseMsg(msg, z.array(SessionViewSchema), 'sessions/list').obj ?? [];
}

// Signing out the browser making the request ends this page's own login.
function leaveIfSignedOut(basePath: string) {
  window.location.assign(basePath || '/');
}

export default function SessionsPage() {
  const { t } = useTranslation();
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  const { datepicker } = useDatepicker();
  const [messageApi, messageContextHolder] = message.useMessage();
  const queryClient = useQueryClient();
  usePageTitle();
  useEffect(() => {
    setMessageInstance(messageApi);
  }, [messageApi]);

  const me = useCurrentAdmin().data;
  const { data: sessions = [], isLoading } = useQuery({
    queryKey: keys.sessions.list(),
    queryFn: fetchSessions,
    refetchInterval: POLL_MS,
  });
  const refresh = () => queryClient.invalidateQueries({ queryKey: keys.sessions.list() });
  const basePath = window.X_UI_BASE_PATH || '/';

  const revoke = useMutation({
    mutationFn: (s: SessionView) => HttpUtil.post(`/panel/api/sessions/revoke/${s.id}`),
    onSuccess: async (msg, s) => {
      if (msg?.success && s.current) leaveIfSignedOut(basePath);
      else await refresh();
    },
  });
  const revokeAll = useMutation({
    mutationFn: (keepCurrent: boolean) =>
      HttpUtil.post('/panel/api/sessions/revokeAll', { keepCurrent }, JSON_HEADERS),
    onSuccess: async (msg, keepCurrent) => {
      if (msg?.success && !keepCurrent) leaveIfSignedOut(basePath);
      else await refresh();
    },
  });

  const columns: TableColumnsType<SessionView> = [
    ...(me?.role === 'owner'
      ? [
          {
            title: t('username'),
            dataIndex: 'username',
            width: 160,
          } satisfies TableColumnsType<SessionView>[number],
        ]
      : []),
    {
      title: t('pages.sessions.address'),
      dataIndex: 'ip',
      width: 170,
      render: (ip: string, s) => (
        <span dir="ltr" className="sessions-ip">
          {ip}
          {s.current && (
            <Tag color="green" className="sessions-current">
              {t('pages.sessions.thisBrowser')}
            </Tag>
          )}
        </span>
      ),
    },
    {
      title: t('pages.sessions.device'),
      dataIndex: 'userAgent',
      ellipsis: true,
      render: (ua: string) => (
        <span dir="ltr" className="sessions-ua">
          {ua || '-'}
        </span>
      ),
    },
    {
      title: t('pages.sessions.signedIn'),
      dataIndex: 'createdAt',
      width: 170,
      render: (ms: number) => (
        <span className="sessions-sub">{IntlUtil.formatDate(ms, datepicker)}</span>
      ),
    },
    {
      title: t('pages.sessions.lastSeen'),
      dataIndex: 'lastSeen',
      width: 170,
      render: (ms: number) => (
        <span className="sessions-sub">{IntlUtil.formatDate(ms, datepicker)}</span>
      ),
    },
    {
      width: 60,
      render: (_, s) => (
        <Popconfirm
          title={t('pages.sessions.endConfirm')}
          okButtonProps={{ danger: true }}
          onConfirm={() => revoke.mutate(s)}
        >
          <Button
            size="small"
            danger
            icon={<DeleteOutlined />}
            aria-label={t('pages.sessions.end')}
          />
        </Popconfirm>
      ),
    },
  ];

  const pageClass = `sessions-page ${isDark ? 'is-dark' : ''} ${isUltra ? 'is-ultra' : ''}`.trim();

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
                <span className="sessions-title">
                  <SafetyCertificateOutlined /> {t('pages.sessions.title')}
                </span>
              }
              extra={
                <Space>
                  <Button onClick={() => revokeAll.mutate(true)}>
                    {t('pages.sessions.endOthers')}
                  </Button>
                  <Popconfirm
                    title={t('pages.sessions.logoutEverywhereConfirm')}
                    okButtonProps={{ danger: true }}
                    onConfirm={() => revokeAll.mutate(false)}
                  >
                    <Button danger icon={<LogoutOutlined />}>
                      {t('pages.sessions.logoutEverywhere')}
                    </Button>
                  </Popconfirm>
                </Space>
              }
            >
              <p className="sessions-hint" dir="auto">
                {t('pages.sessions.hint')}
              </p>
              <Table<SessionView>
                rowKey="id"
                size="small"
                loading={isLoading}
                columns={columns}
                dataSource={sessions}
                pagination={false}
                scroll={{ x: 760 }}
              />
            </Card>
          </Layout.Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  );
}
