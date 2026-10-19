import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Popconfirm, Space, Table, Tag } from 'antd';
import type { TableColumnsType } from 'antd';
import { DeleteOutlined, EditOutlined, PlusOutlined } from '@ant-design/icons';

import { useClientTemplates } from '@/api/queries/useClientTemplates';
import { usePlans } from '@/api/queries/useSales';
import type { Plan } from '@/generated/zod';
import { formatMoney } from '@/lib/money';
import { blankPlan, formToPlan, planToForm } from '@/lib/plan';
import type { PlanFormValues } from '@/schemas/sales';
import PlanFormModal from './PlanFormModal';

const BYTES_PER_GB = 1024 ** 3;

export default function PlansTab() {
  const { t } = useTranslation();
  const { plans, loading, save, saving, remove } = usePlans();
  const { templates } = useClientTemplates(true);
  const templateName = useMemo(
    () => new Map(templates.map((tpl) => [tpl.id, tpl.name])),
    [templates],
  );

  const [editing, setEditing] = useState<PlanFormValues | null>(null);

  async function onSubmit(values: PlanFormValues) {
    const msg = await save(formToPlan(values));
    if (msg?.success) setEditing(null);
  }

  const columns: TableColumnsType<Plan> = [
    {
      title: t('pages.sales.plans.name'),
      dataIndex: 'name',
      render: (name: string, p) => (
        <span dir="auto">
          {name} {!p.enabled && <Tag>{t('pages.sales.plans.disabled')}</Tag>}
        </span>
      ),
    },
    {
      title: t('pages.sales.plans.price'),
      render: (_, p) => formatMoney(p.price, p.currency),
    },
    {
      title: t('pages.sales.plans.quotaGb'),
      render: (_, p) =>
        p.totalGB === 0 ? '∞' : Math.round((p.totalGB / BYTES_PER_GB) * 100) / 100,
    },
    {
      title: t('pages.sales.plans.durationDays'),
      render: (_, p) => (p.durationDays === 0 ? '∞' : p.durationDays),
    },
    {
      title: t('pages.sales.plans.template'),
      render: (_, p) => templateName.get(p.templateId) ?? `#${p.templateId}`,
    },
    {
      width: 90,
      render: (_, p) => (
        <Space size={4}>
          <Button
            size="small"
            icon={<EditOutlined />}
            aria-label={t('edit')}
            onClick={() => setEditing(planToForm(p))}
          />
          <Popconfirm
            title={t('pages.sales.plans.confirmDelete')}
            okButtonProps={{ danger: true }}
            onConfirm={() => remove(p.id)}
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
        <Button
          type="primary"
          icon={<PlusOutlined />}
          onClick={() => setEditing(blankPlan(templates[0]?.id ?? 0))}
        >
          {t('pages.sales.plans.add')}
        </Button>
      </div>
      <Table<Plan>
        rowKey="id"
        size="small"
        loading={loading}
        columns={columns}
        dataSource={plans}
        pagination={false}
        scroll={{ x: 640 }}
      />
      <PlanFormModal
        open={editing !== null}
        initial={editing ?? blankPlan(0)}
        templates={templates}
        saving={saving}
        onClose={() => setEditing(null)}
        onSubmit={onSubmit}
      />
    </>
  );
}
