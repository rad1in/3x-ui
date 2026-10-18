import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Card,
  ConfigProvider,
  Layout,
  Segmented,
  Space,
  Statistic,
  Table,
  Tag,
  message,
} from 'antd';
import type { TableColumnsType } from 'antd';
import { WalletOutlined } from '@ant-design/icons';

import { useAdminList } from '@/api/queries/useAdminList';
import { useCurrentAdmin } from '@/api/queries/useCurrentAdmin';
import { useResellerMe, useResellerReport } from '@/api/queries/useReseller';
import type { ResellerReportRow } from '@/generated/zod';
import { usePageTitle } from '@/hooks/usePageTitle';
import { useTheme } from '@/hooks/useTheme';
import AppSidebar from '@/layouts/AppSidebar';
import { SizeFormatter } from '@/utils';
import { setMessageInstance } from '@/utils/messageBus';
import { formatCredit, REPORT_RANGES, rangeStart, type ReportRange } from './credit';
import ResellerConfigCard from './ResellerConfigCard';
import ResellerCreditModal from './ResellerCreditModal';
import ResellerLedger from './ResellerLedger';
import './ResellerPage.css';

export default function ResellerPage() {
  const { t } = useTranslation();
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  const [messageApi, messageContextHolder] = message.useMessage();
  usePageTitle();
  useEffect(() => {
    setMessageInstance(messageApi);
  }, [messageApi]);

  const role = useCurrentAdmin().data?.role;
  const isReseller = role === 'operator';
  const isOwner = role === 'owner';

  const [range, setRange] = useState<ReportRange>('30');
  const from = useMemo(() => rangeStart(range), [range]);
  const report = useResellerReport(from, 0);
  const mine = useResellerMe(isReseller);
  const admins = useAdminList(isOwner);
  const [managing, setManaging] = useState<number | null>(null);
  const managed = admins.data?.find((a) => a.id === managing);

  const columns: TableColumnsType<ResellerReportRow> = [
    {
      title: t('pages.reseller.report.reseller'),
      dataIndex: 'username',
      render: (name: string) => <span dir="auto">{name}</span>,
    },
    {
      title: t('pages.reseller.credit'),
      dataIndex: 'balance',
      render: (balance: number) => formatCredit(balance),
    },
    { title: t('pages.reseller.report.created'), dataIndex: 'created' },
    { title: t('pages.reseller.report.renewed'), dataIndex: 'renewed' },
    {
      title: t('pages.reseller.report.spent'),
      dataIndex: 'spent',
      render: (spent: number) => formatCredit(spent),
    },
    {
      title: t('pages.reseller.report.traffic'),
      dataIndex: 'bytes',
      render: (bytes: number) => SizeFormatter.sizeFormat(bytes),
    },
    { title: t('pages.reseller.report.days'), dataIndex: 'days' },
    {
      title: t('pages.reseller.report.toppedUp'),
      dataIndex: 'toppedUp',
      render: (sum: number) => formatCredit(sum),
    },
    ...(isOwner
      ? [
          {
            width: 110,
            render: (_: unknown, row: ResellerReportRow) => (
              <Button size="small" onClick={() => setManaging(row.userId)}>
                {t('pages.reseller.manage')}
              </Button>
            ),
          },
        ]
      : []),
  ];

  const pageClass = `reseller-page ${isDark ? 'is-dark' : ''} ${isUltra ? 'is-ultra' : ''}`.trim();

  return (
    <ConfigProvider theme={antdThemeConfig}>
      {messageContextHolder}
      <Layout className={pageClass}>
        <AppSidebar />
        <Layout className="content-shell">
          <Layout.Content className="content-area">
            {isReseller && mine.data && (
              <Card
                size="small"
                className="reseller-card"
                title={
                  <span className="reseller-title">
                    <WalletOutlined /> {t('pages.reseller.mine')}
                  </span>
                }
              >
                <p className="reseller-hint" dir="auto">
                  {t('pages.reseller.hintSelf')}
                </p>
                <Space size="large" wrap className="reseller-stats">
                  <Statistic title={t('pages.reseller.credit')} value={mine.data.balance} />
                  <Tag>
                    {t('pages.reseller.pricePerGb')}: {formatCredit(mine.data.prices.pricePerGb)}
                  </Tag>
                  <Tag>
                    {t('pages.reseller.pricePerDay')}: {formatCredit(mine.data.prices.pricePerDay)}
                  </Tag>
                </Space>
                <ResellerLedger entries={mine.data.entries} loading={mine.isLoading} />
              </Card>
            )}
            {isOwner && <ResellerConfigCard />}
            <Card
              size="small"
              title={
                <span className="reseller-title">
                  <WalletOutlined /> {t('pages.reseller.title')}
                </span>
              }
              extra={
                <Segmented<ReportRange>
                  value={range}
                  options={REPORT_RANGES.map((r) => ({
                    value: r,
                    label: t(`pages.reseller.ranges.${r}`),
                  }))}
                  onChange={setRange}
                />
              }
            >
              {!isReseller && (
                <p className="reseller-hint" dir="auto">
                  {t('pages.reseller.hint')}
                </p>
              )}
              <Table<ResellerReportRow>
                rowKey="userId"
                size="small"
                loading={report.isLoading}
                columns={columns}
                dataSource={report.data?.rows ?? []}
                pagination={false}
                scroll={{ x: 760 }}
              />
            </Card>
            {managed && (
              <ResellerCreditModal
                key={managed.id}
                reseller={managed}
                onClose={() => setManaging(null)}
              />
            )}
          </Layout.Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  );
}
