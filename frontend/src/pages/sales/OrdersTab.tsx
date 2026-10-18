import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Descriptions, Input, Modal, Popconfirm, Select, Space, Table, Tag } from 'antd';
import type { TableColumnsType } from 'antd';
import { CheckOutlined, CloseOutlined, ReloadOutlined } from '@ant-design/icons';

import { ORDER_PAGE_SIZE, useOrderActions, useOrders } from '@/api/queries/useSales';
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

const isOpen = (o: Order) => o.status === 'pending' || o.status === 'review';
const isUndelivered = (o: Order) => o.status === 'paid' && o.fulfilledAt === 0;

export default function OrdersTab() {
  const { t } = useTranslation();
  const [status, setStatus] = useState('');
  const [page, setPage] = useState(1);
  const { data, isLoading } = useOrders(status, page);
  const { approve, reject } = useOrderActions();

  const [rejecting, setRejecting] = useState<Order | null>(null);
  const [reason, setReason] = useState('');

  async function onReject() {
    if (!rejecting) return;
    const msg = await reject({ id: rejecting.id, reason });
    if (msg?.success) {
      setRejecting(null);
      setReason('');
    }
  }

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
    {
      title: t('pages.sales.orders.gateway'),
      dataIndex: 'gateway',
      render: (g: string) => t(`pages.sales.gateways.${g}`, { defaultValue: g }),
    },
    {
      title: t('pages.sales.orders.status'),
      dataIndex: 'status',
      render: (s: string, o) =>
        isUndelivered(o) ? (
          <Tag color="orange">{t('pages.sales.orders.undelivered')}</Tag>
        ) : (
          <Tag color={STATUS_COLORS[s]}>{t(`pages.sales.orders.statuses.${s}`)}</Tag>
        ),
    },
    {
      title: t('pages.sales.orders.created'),
      dataIndex: 'createdAt',
      render: (ms: number) => new Date(ms).toLocaleString(),
    },
    {
      width: 110,
      render: (_, o) => (
        <Space size={4}>
          {isOpen(o) && (
            <>
              <Popconfirm
                title={t('pages.sales.orders.confirmApprove')}
                onConfirm={() => approve(o.id)}
              >
                <Button
                  size="small"
                  type="primary"
                  icon={<CheckOutlined />}
                  aria-label={t('pages.sales.orders.approve')}
                />
              </Popconfirm>
              <Button
                size="small"
                danger
                icon={<CloseOutlined />}
                aria-label={t('pages.sales.orders.reject')}
                onClick={() => setRejecting(o)}
              />
            </>
          )}
          {isUndelivered(o) && (
            <Button
              size="small"
              icon={<ReloadOutlined />}
              aria-label={t('pages.sales.orders.retry')}
              onClick={() => approve(o.id)}
            />
          )}
        </Space>
      ),
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
        scroll={{ x: 820 }}
        expandable={{
          expandedRowRender: (o) => (
            <Descriptions size="small" column={1}>
              <Descriptions.Item label={t('pages.sales.orders.contact')}>
                <span dir="auto">{o.contact || '-'}</span>
              </Descriptions.Item>
              <Descriptions.Item label={t('pages.sales.orders.receiptRef')}>
                <span dir="auto">{o.receiptRef || '-'}</span>
              </Descriptions.Item>
              <Descriptions.Item label={t('pages.sales.orders.receiptNote')}>
                <span dir="auto">{o.receiptNote || '-'}</span>
              </Descriptions.Item>
              {o.rejectReason && (
                <Descriptions.Item label={t('pages.sales.orders.rejectReason')}>
                  <span dir="auto">{o.rejectReason}</span>
                </Descriptions.Item>
              )}
            </Descriptions>
          ),
        }}
        pagination={{
          current: page,
          pageSize: ORDER_PAGE_SIZE,
          total: data?.total ?? 0,
          showSizeChanger: false,
          onChange: setPage,
        }}
      />
      <Modal
        open={rejecting !== null}
        title={t('pages.sales.orders.reject')}
        okText={t('pages.sales.orders.reject')}
        okButtonProps={{ danger: true }}
        cancelText={t('close')}
        onOk={onReject}
        onCancel={() => setRejecting(null)}
        destroyOnHidden
      >
        <Input.TextArea
          rows={3}
          maxLength={300}
          value={reason}
          placeholder={t('pages.sales.orders.rejectReasonHint')}
          onChange={(e) => setReason(e.target.value)}
        />
      </Modal>
    </>
  );
}
