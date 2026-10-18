import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Select, Table, Tag } from 'antd';
import type { TableColumnsType } from 'antd';

import { ORDER_PAGE_SIZE, useOrders } from '@/api/queries/useSales';
import type { Order } from '@/generated/zod';
import { formatMoney } from '@/lib/money';
import { ORDER_STATUSES } from '@/schemas/sales';

const STATUS_COLORS: Record<string, string> = {
  pending: 'default',
  review: 'gold',
  paid: 'green',
  rejected: 'red',
  failed: 'red',
  canceled: 'default',
};

export default function OrdersTab() {
  const { t } = useTranslation();
  const [status, setStatus] = useState('');
  const [page, setPage] = useState(1);
  const { data, isLoading } = useOrders(status, page);

  const columns: TableColumnsType<Order> = [
    { title: '#', dataIndex: 'id', width: 70 },
    {
      title: t('pages.sales.orders.plan'),
      dataIndex: 'planName',
      render: (name: string) => <span dir="auto">{name}</span>,
    },
    {
      title: t('pages.sales.orders.client'),
      render: (_, o) => (
        <span dir="auto">
          {o.clientEmail} <Tag>{t(`pages.sales.orders.kinds.${o.kind}`)}</Tag>
        </span>
      ),
    },
    { title: t('pages.sales.orders.amount'), render: (_, o) => formatMoney(o.amount, o.currency) },
    { title: t('pages.sales.orders.gateway'), dataIndex: 'gateway' },
    {
      title: t('pages.sales.orders.status'),
      dataIndex: 'status',
      render: (s: string) => (
        <Tag color={STATUS_COLORS[s]}>{t(`pages.sales.orders.statuses.${s}`)}</Tag>
      ),
    },
    {
      title: t('pages.sales.orders.created'),
      dataIndex: 'createdAt',
      render: (ms: number) => new Date(ms).toLocaleString(),
    },
  ];

  return (
    <>
      <div className="sales-toolbar">
        <Select
          value={status}
          style={{ minWidth: 180 }}
          onChange={(v) => {
            setStatus(v);
            setPage(1);
          }}
          options={[
            { value: '', label: t('pages.sales.orders.allStatuses') },
            ...ORDER_STATUSES.map((s) => ({
              value: s,
              label: t(`pages.sales.orders.statuses.${s}`),
            })),
          ]}
        />
      </div>
      <Table<Order>
        rowKey="id"
        size="small"
        loading={isLoading}
        columns={columns}
        dataSource={data?.items ?? []}
        scroll={{ x: 720 }}
        pagination={{
          current: page,
          pageSize: ORDER_PAGE_SIZE,
          total: data?.total ?? 0,
          showSizeChanger: false,
          onChange: setPage,
        }}
      />
    </>
  );
}
