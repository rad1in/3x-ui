import { useTranslation } from 'react-i18next';
import { Card, Empty, Tag } from 'antd';

import type { ShopPlan } from '@/generated/zod';
import { formatMoney } from '@/lib/money';

const BYTES_PER_GB = 1024 ** 3;

interface PlanListProps {
  plans: ShopPlan[];
  selected: number | null;
  onSelect: (id: number) => void;
}

export default function PlanList({ plans, selected, onSelect }: PlanListProps) {
  const { t } = useTranslation();
  if (plans.length === 0) return <Empty description={t('shop.noPlans')} />;
  return (
    <div className="shop-plans">
      {plans.map((p) => (
        <Card
          key={p.id}
          size="small"
          hoverable
          className={p.id === selected ? 'shop-plan is-selected' : 'shop-plan'}
          onClick={() => onSelect(p.id)}
        >
          <div className="shop-plan-name" dir="auto">
            {p.name}
          </div>
          <div className="shop-plan-price">{formatMoney(p.price, p.currency)}</div>
          {p.description && (
            <div className="shop-plan-desc" dir="auto">
              {p.description}
            </div>
          )}
          <div className="shop-plan-facts">
            <Tag>
              {p.totalGB === 0
                ? t('shop.unlimitedData')
                : t('shop.dataGb', { gb: Math.round((p.totalGB / BYTES_PER_GB) * 100) / 100 })}
            </Tag>
            <Tag>
              {p.durationDays === 0 ? t('shop.noExpiry') : t('shop.days', { days: p.durationDays })}
            </Tag>
            {p.limitIp > 0 && <Tag>{t('shop.devices', { count: p.limitIp })}</Tag>}
          </div>
        </Card>
      ))}
    </div>
  );
}
