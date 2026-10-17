import { useTranslation } from 'react-i18next';
import { Alert, Card, Progress } from 'antd';

import { useHostedStatus } from '@/api/queries/useHostedStatus';
import { useDatepicker } from '@/hooks/useDatepicker';
import { IntlUtil, SizeFormatter } from '@/utils';

const BLOCKED_KEYS: Record<string, string> = {
  traffic: 'pages.index.hosted.blockedTraffic',
  expired: 'pages.index.hosted.blockedExpired',
  'no-limits': 'pages.index.hosted.blockedNoLimits',
};

function UsageRow({
  label,
  used,
  max,
  format,
}: {
  label: string;
  used: number;
  max: number;
  format: (n: number) => string;
}) {
  const { t } = useTranslation();
  const percent = max > 0 ? Math.min(100, (used / max) * 100) : 0;
  return (
    <div className="ov-hosted-row">
      <div className="ov-hosted-row-head">
        <span>{label}</span>
        <span className="ov-hosted-row-value">
          {max > 0 ? `${format(used)} / ${format(max)}` : `${format(used)} · ${t('unlimited')}`}
        </span>
      </div>
      <Progress
        percent={percent}
        showInfo={false}
        size="small"
        status={max > 0 && used >= max ? 'exception' : 'normal'}
      />
    </div>
  );
}

export default function HostedUsageCard() {
  const { t } = useTranslation();
  const { datepicker } = useDatepicker();
  const { data } = useHostedStatus({ poll: true });
  if (!data?.enabled) return null;
  const { limits } = data;
  const count = (n: number) => String(n);
  return (
    <Card hoverable styles={{ body: { padding: 0 } }}>
      <div className="ov-hosted" data-testid="hosted-usage">
        <div className="ov-kicker">{t('pages.index.hosted.title')}</div>
        {data.blocked && (
          <Alert
            type="error"
            showIcon
            title={t(BLOCKED_KEYS[data.blocked] ?? 'pages.index.hosted.blockedNoLimits')}
          />
        )}
        <div className="ov-hosted-grid">
          <UsageRow
            label={t('menu.inbounds')}
            used={data.inbounds}
            max={limits.maxInbounds}
            format={count}
          />
          <UsageRow
            label={t('clients')}
            used={data.clients}
            max={limits.maxClients}
            format={count}
          />
          <UsageRow
            label={t('usage')}
            used={data.usedBytes}
            max={limits.trafficBytes}
            format={SizeFormatter.sizeFormat}
          />
        </div>
        <div className="ov-hosted-meta">
          <span>
            {t('pages.index.hosted.expires')}:{' '}
            {limits.expiresAt > 0
              ? IntlUtil.formatDate(limits.expiresAt, datepicker)
              : t('indefinite')}
          </span>
          {(limits.portMin > 0 || limits.portMax > 0) && (
            <span>
              {t('pages.index.hosted.ports')}: {limits.portMin}–{limits.portMax}
            </span>
          )}
        </div>
      </div>
    </Card>
  );
}
