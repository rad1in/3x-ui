import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Card,
  ConfigProvider,
  Layout,
  Popconfirm,
  Progress,
  Space,
  Table,
  Tag,
  Tooltip,
  Typography,
  message,
} from 'antd';
import type { TableColumnsType } from 'antd';
import {
  AppstoreOutlined,
  DeleteOutlined,
  EditOutlined,
  ExportOutlined,
  PlusOutlined,
  UndoOutlined,
} from '@ant-design/icons';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { z } from 'zod';

import { keys } from '@/api/queryKeys';
import EmptyState from '@/components/feedback/EmptyState';
import { HostedPanelViewSchema, type HostedPanelView } from '@/generated/zod';
import { useDatepicker } from '@/hooks/useDatepicker';
import { usePageTitle } from '@/hooks/usePageTitle';
import { useTheme } from '@/hooks/useTheme';
import AppSidebar from '@/layouts/AppSidebar';
import { gbToBytes, type HostedPanelFormValues } from '@/schemas/hostedPanel';
import { HttpUtil, IntlUtil, SizeFormatter } from '@/utils';
import { setMessageInstance } from '@/utils/messageBus';
import { parseMsg } from '@/utils/zodValidate';
import PanelFormModal from './PanelFormModal';
import {
  hostedPanelState,
  hostedPanelUrl,
  suggestHostedPorts,
  type HostedPanelState,
} from './panelState';
import './ResellerPage.css';

const JSON_HEADERS = { headers: { 'Content-Type': 'application/json' } } as const;
const POLL_MS = 10_000;

async function fetchPanels(): Promise<HostedPanelView[]> {
  const msg = await HttpUtil.get('/panel/api/hostedPanels/list', undefined, { silent: true });
  if (!msg?.success) throw new Error(msg?.msg || 'Failed to load panels');
  return parseMsg(msg, z.array(HostedPanelViewSchema), 'hostedPanels/list').obj ?? [];
}

const STATE_TAGS: Record<HostedPanelState, { color: string; key: string }> = {
  running: { color: 'success', key: 'pages.hostedPanels.stateRunning' },
  starting: { color: 'processing', key: 'pages.hostedPanels.stateStarting' },
  suspended: { color: 'warning', key: 'pages.hostedPanels.stateSuspended' },
  crashed: { color: 'error', key: 'pages.hostedPanels.stateCrashed' },
  disabled: { color: 'default', key: 'pages.hostedPanels.stateDisabled' },
};

const BLOCKED_KEYS: Record<string, string> = {
  traffic: 'pages.index.hosted.blockedTraffic',
  expired: 'pages.index.hosted.blockedExpired',
  'no-limits': 'pages.index.hosted.blockedNoLimits',
};

function CountCell({ used, max }: { used: number; max: number }) {
  const { t } = useTranslation();
  return (
    <span className={`hosted-count ${max > 0 && used >= max ? 'is-full' : ''}`} dir="ltr">
      {used} / {max > 0 ? max : t('unlimited')}
    </span>
  );
}

export default function ResellerPage() {
  const { t } = useTranslation();
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  const { datepicker } = useDatepicker();
  const [messageApi, messageContextHolder] = message.useMessage();
  const queryClient = useQueryClient();
  usePageTitle();
  useEffect(() => {
    setMessageInstance(messageApi);
  }, [messageApi]);

  const { data: panels = [], isLoading } = useQuery({
    queryKey: keys.hostedPanels.list(),
    queryFn: fetchPanels,
    refetchInterval: POLL_MS,
  });

  const [editing, setEditing] = useState<HostedPanelView | null>(null);
  const [modalOpen, setModalOpen] = useState(false);
  // Fixed when the dialog opens: the list polls, and a fresh suggestion would reset the form.
  const [suggested, setSuggested] = useState(() => suggestHostedPorts([], []));

  const refresh = () => queryClient.invalidateQueries({ queryKey: keys.hostedPanels.list() });

  const save = useMutation({
    mutationFn: ({ trafficGB, ...values }: HostedPanelFormValues) => {
      const body = { ...values, trafficBytes: gbToBytes(trafficGB) };
      return editing
        ? HttpUtil.post(`/panel/api/hostedPanels/update/${editing.id}`, body, JSON_HEADERS)
        : HttpUtil.post('/panel/api/hostedPanels/add', body, JSON_HEADERS);
    },
    onSuccess: async (msg) => {
      if (!msg?.success) return;
      setModalOpen(false);
      await refresh();
    },
  });
  const remove = useMutation({
    mutationFn: (id: number) => HttpUtil.post(`/panel/api/hostedPanels/del/${id}`),
    onSuccess: refresh,
  });
  const resetUsage = useMutation({
    mutationFn: (id: number) => HttpUtil.post(`/panel/api/hostedPanels/resetUsage/${id}`),
    onSuccess: refresh,
  });

  const openEditor = (panel: HostedPanelView | null) => {
    setSuggested(suggestHostedPorts(panels, [Number(window.location.port) || 0]));
    setEditing(panel);
    setModalOpen(true);
  };

  const columns: TableColumnsType<HostedPanelView> = [
    {
      title: t('pages.hostedPanels.name'),
      dataIndex: 'name',
      render: (name: string, p) => (
        <div>
          <div className="hosted-name" dir="auto">
            {name}
          </div>
          <div className="hosted-sub">{p.username}</div>
        </div>
      ),
    },
    {
      title: t('status'),
      width: 130,
      render: (_, p) => {
        const state = hostedPanelState(p);
        const tag = <Tag color={STATE_TAGS[state].color}>{t(STATE_TAGS[state].key)}</Tag>;
        const reason =
          state === 'suspended'
            ? t(BLOCKED_KEYS[p.status.blocked] ?? 'pages.index.hosted.blockedNoLimits')
            : state === 'crashed'
              ? p.process.lastError
              : '';
        return reason ? <Tooltip title={reason}>{tag}</Tooltip> : tag;
      },
    },
    {
      title: t('pages.hostedPanels.address'),
      render: (_, p) => {
        const url = hostedPanelUrl(p, window.location.hostname);
        return (
          <div>
            <Typography.Link
              href={url}
              target="_blank"
              rel="noopener noreferrer"
              className="hosted-url"
              dir="ltr"
              copyable={{ text: url }}
            >
              :{p.webPort}
              {p.webBasePath} <ExportOutlined />
            </Typography.Link>
            <div className="hosted-sub">
              {t('pages.hostedPanels.portRange')}: <bdi>{`${p.portMin}–${p.portMax}`}</bdi>
            </div>
          </div>
        );
      },
    },
    {
      title: t('usage'),
      width: 200,
      render: (_, p) => {
        const used = p.status.usedBytes;
        const max = p.trafficBytes;
        return (
          <div className="hosted-usage">
            <span dir="ltr">
              {SizeFormatter.sizeFormat(used)}
              {' / '}
              {max > 0 ? SizeFormatter.sizeFormat(max) : t('unlimited')}
            </span>
            {max > 0 && (
              <Progress
                percent={Math.min(100, (used / max) * 100)}
                showInfo={false}
                size="small"
                status={used >= max ? 'exception' : 'normal'}
              />
            )}
          </div>
        );
      },
    },
    {
      title: t('menu.inbounds'),
      width: 100,
      render: (_, p) => <CountCell used={p.status.inbounds} max={p.maxInbounds} />,
    },
    {
      title: t('clients'),
      width: 100,
      render: (_, p) => <CountCell used={p.status.clients} max={p.maxClients} />,
    },
    {
      title: t('pages.hostedPanels.expires'),
      dataIndex: 'expiresAt',
      width: 150,
      render: (ms: number) => (
        <span className="hosted-sub">
          {ms > 0 ? IntlUtil.formatDate(ms, datepicker) : t('indefinite')}
        </span>
      ),
    },
    {
      width: 120,
      render: (_, p) => (
        <Space size={4}>
          <Button
            size="small"
            icon={<EditOutlined />}
            aria-label={t('edit')}
            onClick={() => openEditor(p)}
          />
          <Popconfirm
            title={t('pages.hostedPanels.resetUsageConfirm')}
            onConfirm={() => resetUsage.mutate(p.id)}
          >
            <Tooltip title={t('pages.hostedPanels.resetUsage')}>
              <Button
                size="small"
                icon={<UndoOutlined />}
                aria-label={t('pages.hostedPanels.resetUsage')}
              />
            </Tooltip>
          </Popconfirm>
          <Popconfirm
            title={t('pages.hostedPanels.deleteConfirm')}
            description={t('pages.hostedPanels.deleteWarning')}
            okButtonProps={{ danger: true }}
            onConfirm={() => remove.mutate(p.id)}
          >
            <Button size="small" danger icon={<DeleteOutlined />} aria-label={t('delete')} />
          </Popconfirm>
        </Space>
      ),
    },
  ];

  const pageClass = `panels-page ${isDark ? 'is-dark' : ''} ${isUltra ? 'is-ultra' : ''}`.trim();

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
                <span className="hosted-title">
                  <AppstoreOutlined /> {t('pages.hostedPanels.title')}
                </span>
              }
              extra={
                <Button type="primary" icon={<PlusOutlined />} onClick={() => openEditor(null)}>
                  {t('pages.hostedPanels.add')}
                </Button>
              }
            >
              <p className="hosted-hint" dir="auto">
                {t('pages.hostedPanels.hint')}
              </p>
              <Table<HostedPanelView>
                rowKey="id"
                size="small"
                loading={isLoading}
                columns={columns}
                dataSource={panels}
                pagination={false}
                scroll={{ x: 1000 }}
                locale={{
                  emptyText: (
                    <EmptyState
                      icon={<AppstoreOutlined />}
                      title={t('pages.hostedPanels.empty')}
                      description={t('pages.hostedPanels.emptyHint')}
                      action={
                        <Button
                          type="primary"
                          icon={<PlusOutlined />}
                          onClick={() => openEditor(null)}
                        >
                          {t('pages.hostedPanels.add')}
                        </Button>
                      }
                    />
                  ),
                }}
              />
            </Card>
            <PanelFormModal
              open={modalOpen}
              panel={editing}
              suggested={suggested}
              saving={save.isPending}
              onClose={() => setModalOpen(false)}
              onSubmit={(values) => save.mutate(values)}
            />
          </Layout.Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  );
}
