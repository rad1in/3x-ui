import { useTranslation } from 'react-i18next';
import { Card, Tag } from 'antd';

import { useHealth } from '@/api/queries/useHealth';

const STATUS_COLOR: Record<string, string> = {
  ok: 'success',
  warn: 'warning',
  fail: 'error',
  skip: 'default',
};

export default function HealthCard() {
  const { t } = useTranslation();
  const { data } = useHealth();
  if (!data) return null;

  const names: Record<string, string> = {
    database: t('pages.index.healthCheckDatabase'),
    xray: t('pages.index.healthCheckXray'),
    nodes: t('pages.index.healthCheckNodes'),
    disk: t('pages.index.healthCheckDisk'),
    cert: t('pages.index.healthCheckCert'),
  };
  const statuses: Record<string, string> = {
    ok: t('pages.index.healthOk'),
    warn: t('pages.index.healthWarn'),
    fail: t('pages.index.healthFail'),
    skip: t('pages.index.healthSkip'),
  };

  return (
    <Card hoverable styles={{ body: { padding: 0 } }} data-testid="health-card">
      <div className="ov-insight-head">
        <div className="ov-kicker">{t('pages.index.healthTitle')}</div>
        <Tag color={STATUS_COLOR[data.status]}>{statuses[data.status]}</Tag>
      </div>
      <ul className="ov-insight-list" style={{ padding: '0 20px 12px', margin: 0 }}>
        {data.checks.map((c) => (
          <li key={c.name} className="ov-insight-row">
            <span className="ov-insight-email">{names[c.name] ?? c.name}</span>
            <Tag color={STATUS_COLOR[c.status]}>{statuses[c.status]}</Tag>
            {c.detail && <span className="ov-insight-meta">{c.detail}</span>}
          </li>
        ))}
      </ul>
    </Card>
  );
}
