import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Popconfirm, Space, Table, Tag } from 'antd';
import type { TableColumnsType } from 'antd';
import { DeleteOutlined, EditOutlined, PlusOutlined } from '@ant-design/icons';

import { useDiscounts, usePlans } from '@/api/queries/useSales';
import type { DiscountCode } from '@/generated/zod';
import { blankDiscount, discountToForm, formToDiscount } from '@/lib/discount';
import { formatMoney } from '@/lib/money';
import type { DiscountFormValues } from '@/schemas/sales';
import DiscountFormModal from './DiscountFormModal';

export default function DiscountsTab() {
  const { t } = useTranslation();
  const { discounts, loading, save, saving, remove } = useDiscounts();
  const { plans } = usePlans();
  const [editing, setEditing] = useState<DiscountFormValues | null>(null);

  async function onSubmit(values: DiscountFormValues) {
    const msg = await save(formToDiscount(values));
    if (msg?.success) setEditing(null);
  }

  const columns: TableColumnsType<DiscountCode> = [
    {
      title: t('pages.sales.discounts.code'),
      dataIndex: 'code',
      render: (code: string, d) => (
        <span dir="ltr">
          {code} {!d.enabled && <Tag>{t('pages.sales.plans.disabled')}</Tag>}
        </span>
      ),
    },
    {
      title: t('pages.sales.discounts.value'),
      render: (_, d) => (d.kind === 'percent' ? `${d.value}%` : formatMoney(d.value, d.currency)),
    },
    {
      title: t('pages.sales.discounts.uses'),
      render: (_, d) => (d.maxUses > 0 ? `${d.uses} / ${d.maxUses}` : d.uses),
    },
    {
      title: t('pages.sales.discounts.expiresAt'),
      dataIndex: 'expiresAt',
      render: (ms: number) => (ms > 0 ? new Date(ms).toLocaleString() : '-'),
    },
    {
      width: 90,
      render: (_, d) => (
        <Space size={4}>
          <Button
            size="small"
            icon={<EditOutlined />}
            aria-label={t('edit')}
            onClick={() => setEditing(discountToForm(d))}
          />
          <Popconfirm
            title={t('pages.sales.discounts.confirmDelete')}
            okButtonProps={{ danger: true }}
            onConfirm={() => remove(d.id)}
          >
            <Button size="small" danger icon={<DeleteOutlined />} aria-label={t('delete')} />
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <>
      <div className="sales-toolbar">
        <Button type="primary" icon={<PlusOutlined />} onClick={() => setEditing(blankDiscount())}>
          {t('pages.sales.discounts.add')}
        </Button>
      </div>
      <Table<DiscountCode>
        rowKey="id"
        size="small"
        loading={loading}
        columns={columns}
        dataSource={discounts}
        pagination={false}
        scroll={{ x: 640 }}
      />
      <DiscountFormModal
        open={editing !== null}
        initial={editing ?? blankDiscount()}
        plans={plans}
        saving={saving}
        onClose={() => setEditing(null)}
        onSubmit={onSubmit}
      />
    </>
  );
}
