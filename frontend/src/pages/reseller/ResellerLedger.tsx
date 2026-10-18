import { useTranslation } from 'react-i18next';
import { Table, Tag } from 'antd';
import type { TableColumnsType } from 'antd';

import type { ResellerEntry } from '@/generated/zod';
import { SizeFormatter } from '@/utils';
import { formatCredit } from './credit';

const KIND_COLORS: Record<string, string> = { topup: 'green', create: 'blue', renew: 'gold' };

export default function ResellerLedger({
  entries,
  loading,
}: {
  entries: ResellerEntry[];
  loading?: boolean;
}) {
  const { t } = useTranslation();
  const columns: TableColumnsType<ResellerEntry> = [
    {
      title: t('pages.reseller.ledger.time'),
      dataIndex: 'createdAt',
      render: (ms: number) => new Date(ms).toLocaleString(),
    },
    {
      title: t('pages.reseller.ledger.kind'),
      dataIndex: 'kind',
      render: (kind: string) => (
        <Tag color={KIND_COLORS[kind]}>
          {t(`pages.reseller.kinds.${kind}`, { defaultValue: kind })}
        </Tag>
      ),
    },
    {
      title: t('pages.reseller.ledger.amount'),
      dataIndex: 'amount',
      render: (amount: number) => formatCredit(amount),
    },
    {
      title: t('pages.reseller.ledger.client'),
      dataIndex: 'clientEmail',
      render: (email: string, e) => (
        <span dir="auto">
          {email}
          {e.note && <span className="reseller-sub"> {e.note}</span>}
        </span>
      ),
    },
    {
      title: t('pages.reseller.report.traffic'),
      dataIndex: 'bytes',
      render: (bytes: number) => (bytes > 0 ? SizeFormatter.sizeFormat(bytes) : ''),
    },
    {
      title: t('pages.reseller.report.days'),
      dataIndex: 'days',
      render: (days: number) => (days > 0 ? days : ''),
    },
  ];
  return (
    <Table<ResellerEntry>
      rowKey="id"
      size="small"
      loading={loading}
      columns={columns}
      dataSource={entries}
      pagination={{ pageSize: 10, hideOnSinglePage: true }}
      scroll={{ x: 560 }}
    />
  );
}
