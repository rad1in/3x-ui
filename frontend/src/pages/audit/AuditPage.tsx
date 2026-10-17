import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Card,
  ConfigProvider,
  Input,
  Layout,
  Segmented,
  Space,
  Table,
  Tag,
  Tooltip,
  message,
} from 'antd';
import type { TableColumnsType } from 'antd';
import {
  CheckCircleFilled,
  CloseCircleFilled,
  HistoryOutlined,
  LoginOutlined,
} from '@ant-design/icons';
import { keepPreviousData, useQuery } from '@tanstack/react-query';

import { keys } from '@/api/queryKeys';
import { AuditLogPageSchema, type AuditLog } from '@/generated/zod';
import { useTheme } from '@/hooks/useTheme';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import { usePageTitle } from '@/hooks/usePageTitle';
import AppSidebar from '@/layouts/AppSidebar';
import EmptyState from '@/components/feedback/EmptyState';
import { HttpUtil, IntlUtil } from '@/utils';
import { setMessageInstance } from '@/utils/messageBus';
import { parseMsg } from '@/utils/zodValidate';
import { useDatepicker } from '@/hooks/useDatepicker';
import { AUDIT_LOGIN_ACTION, describeAuditAction } from './auditAction';
import './AuditPage.css';

const PAGE_SIZE = 50;
const POLL_MS = 15_000;

type ResultFilter = '' | 'ok' | 'fail';

interface AuditQuery {
  page: number;
  search: string;
  result: ResultFilter;
}

async function fetchAudit(q: AuditQuery) {
  const qs = new URLSearchParams({ page: String(q.page), pageSize: String(PAGE_SIZE) });
  if (q.search) qs.set('search', q.search);
  if (q.result) qs.set('result', q.result);
  const msg = await HttpUtil.get(`/panel/api/audit/list?${qs}`, undefined, { silent: true });
  if (!msg?.success) throw new Error(msg?.msg || 'Failed to load activity');
  const validated = parseMsg(msg, AuditLogPageSchema, 'audit/list');
  return validated.obj ?? { items: [], total: 0 };
}

const SECTION_LABEL_KEYS: Record<string, string> = {
  inbounds: 'menu.inbounds',
  clients: 'menu.clients',
  nodes: 'menu.nodes',
  hosts: 'menu.hosts',
  setting: 'menu.settings',
  xray: 'menu.xray',
  server: 'pages.audit.sectionServer',
  'sub-balancers': 'pages.settings.subBalancers.menu',
  [AUDIT_LOGIN_ACTION]: 'pages.audit.login',
};

export default function AuditPage() {
  const { t } = useTranslation();
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  const { isMobile } = useMediaQuery();
  const { datepicker } = useDatepicker();
  const [messageApi, messageContextHolder] = message.useMessage();
  usePageTitle();
  useEffect(() => {
    setMessageInstance(messageApi);
  }, [messageApi]);

  const [searchInput, setSearchInput] = useState('');
  const [query, setQuery] = useState<AuditQuery>({ page: 1, search: '', result: '' });
  useEffect(() => {
    const timer = window.setTimeout(
      () =>
        setQuery((q) =>
          q.search === searchInput.trim() ? q : { ...q, page: 1, search: searchInput.trim() },
        ),
      300,
    );
    return () => window.clearTimeout(timer);
  }, [searchInput]);

  const { data, isFetching } = useQuery({
    queryKey: keys.audit.list(query),
    queryFn: () => fetchAudit(query),
    placeholderData: keepPreviousData,
    refetchInterval: POLL_MS,
  });

  const columns = useMemo<TableColumnsType<AuditLog>>(
    () => [
      {
        title: t('pages.audit.time'),
        dataIndex: 'createdAt',
        width: 190,
        render: (ms: number) => (
          <Tooltip title={IntlUtil.formatRelativeTime(ms)}>
            <span className="audit-time">{IntlUtil.formatDate(ms, datepicker)}</span>
          </Tooltip>
        ),
      },
      {
        title: t('pages.audit.actor'),
        dataIndex: 'actor',
        width: 170,
        render: (actor: string) => <span className="audit-actor">{actor || '—'}</span>,
      },
      {
        title: t('pages.audit.action'),
        dataIndex: 'action',
        render: (action: string) => {
          const { section, operation } = describeAuditAction(action);
          const labelKey = SECTION_LABEL_KEYS[section];
          return (
            <Space size={6} wrap>
              <Tag icon={section === AUDIT_LOGIN_ACTION ? <LoginOutlined /> : undefined}>
                {labelKey ? t(labelKey) : section}
              </Tag>
              {operation && <span className="audit-op">{operation}</span>}
            </Space>
          );
        },
      },
      {
        title: t('pages.audit.target'),
        dataIndex: 'target',
        render: (target: string) => <span className="audit-target">{target}</span>,
      },
      {
        title: 'IP',
        dataIndex: 'ip',
        width: 150,
        render: (ip: string) => <span className="audit-ip">{ip}</span>,
      },
      {
        title: t('pages.audit.result'),
        dataIndex: 'success',
        width: 110,
        render: (ok: boolean) =>
          ok ? (
            <Tag color="success" icon={<CheckCircleFilled />}>
              {t('success')}
            </Tag>
          ) : (
            <Tag color="error" icon={<CloseCircleFilled />}>
              {t('fail')}
            </Tag>
          ),
      },
    ],
    [t, datepicker],
  );

  const pageClass = `audit-page ${isDark ? 'is-dark' : ''} ${isUltra ? 'is-ultra' : ''}`.trim();

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
                <div className="audit-toolbar">
                  <span className="audit-title">
                    <HistoryOutlined /> {t('pages.audit.title')}
                  </span>
                  <Space wrap>
                    <Input.Search
                      allowClear
                      value={searchInput}
                      placeholder={t('pages.audit.searchPlaceholder')}
                      onChange={(e) => setSearchInput(e.target.value)}
                      style={{ width: isMobile ? 180 : 280 }}
                    />
                    <Segmented<ResultFilter>
                      value={query.result}
                      onChange={(result) => setQuery((q) => ({ ...q, page: 1, result }))}
                      options={[
                        { value: '', label: t('all') },
                        { value: 'ok', label: t('success') },
                        { value: 'fail', label: t('fail') },
                      ]}
                    />
                  </Space>
                </div>
              }
            >
              <p className="audit-hint">{t('pages.audit.hint')}</p>
              <Table<AuditLog>
                rowKey="id"
                size="small"
                columns={columns}
                dataSource={data?.items ?? []}
                loading={isFetching && !data}
                scroll={{ x: 900 }}
                pagination={{
                  current: query.page,
                  pageSize: PAGE_SIZE,
                  total: data?.total ?? 0,
                  showSizeChanger: false,
                  onChange: (page) => setQuery((q) => ({ ...q, page })),
                }}
                locale={{
                  emptyText: (
                    <EmptyState icon={<HistoryOutlined />} title={t('pages.audit.empty')} />
                  ),
                }}
              />
            </Card>
          </Layout.Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  );
}
