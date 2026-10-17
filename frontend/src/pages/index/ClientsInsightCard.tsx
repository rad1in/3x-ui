import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';
import { Card, Progress } from 'antd';
import { useQuery } from '@tanstack/react-query';

import { keys } from '@/api/queryKeys';
import { fetchClientPage, type ClientQueryParams, type ClientRecord } from '@/hooks/useClients';
import { useTheme } from '@/hooks/useTheme';
import { computeTrafficDisplay } from '@/lib/clients/traffic-display';
import { IntlUtil, SizeFormatter } from '@/utils';

const LIST_SIZE = 5;
const POLL_MS = 15_000;

const TOP_USAGE_QUERY: ClientQueryParams = {
  page: 1,
  pageSize: LIST_SIZE,
  sort: 'traffic',
  order: 'descend',
};
// Live clients only; the server sorts never-expiring rows last and the card drops them.
const EXPIRING_QUERY: ClientQueryParams = {
  page: 1,
  pageSize: LIST_SIZE,
  filter: 'active,expiring',
  sort: 'expiryTime',
  order: 'ascend',
};

function useInsightPage(params: ClientQueryParams) {
  return useQuery({
    queryKey: keys.clients.list(params),
    queryFn: () => fetchClientPage(params),
    refetchInterval: POLL_MS,
  });
}

function InsightRow({ client, isDark }: { client: ClientRecord; isDark: boolean }) {
  const { t } = useTranslation();
  const traffic = computeTrafficDisplay(
    {
      up: client.traffic?.up ?? 0,
      down: client.traffic?.down ?? 0,
      total: client.totalGB ?? 0,
      enabled: client.enable ?? true,
      trafficDiff: 0,
    },
    isDark,
  );
  const expiry = client.expiryTime ? IntlUtil.formatRelativeTime(client.expiryTime) : '';
  return (
    <li className="ov-insight-row">
      <Link className="ov-insight-email" to={`/clients?search=${encodeURIComponent(client.email)}`}>
        {client.email}
      </Link>
      <span className="ov-insight-meta">
        {SizeFormatter.sizeFormat(traffic.used)}
        {traffic.isUnlimited
          ? ` · ${t('unlimited')}`
          : ` / ${SizeFormatter.sizeFormat(client.totalGB ?? 0)}`}
        {expiry && ` · ${expiry}`}
      </span>
      <Progress
        className="ov-insight-bar"
        percent={traffic.isUnlimited ? 0 : traffic.percent}
        showInfo={false}
        size="small"
        strokeColor={traffic.strokeColor}
        status={traffic.status}
      />
    </li>
  );
}

function InsightList({
  title,
  testId,
  clients,
  isDark,
}: {
  title: string;
  testId: string;
  clients: ClientRecord[] | undefined;
  isDark: boolean;
}) {
  const { t } = useTranslation();
  return (
    <div className="ov-insight-col">
      <div className="ov-kicker">{title}</div>
      <ul className="ov-insight-list" data-testid={testId}>
        {clients?.length ? (
          clients.map((c) => <InsightRow key={c.email} client={c} isDark={isDark} />)
        ) : (
          <li className="ov-insight-empty">{t('noData')}</li>
        )}
      </ul>
    </div>
  );
}

export default function ClientsInsightCard() {
  const { t } = useTranslation();
  const { isDark } = useTheme();
  const top = useInsightPage(TOP_USAGE_QUERY);
  const expiring = useInsightPage(EXPIRING_QUERY);
  const expiringSoon = expiring.data?.items.filter((c) => (c.expiryTime ?? 0) > 0);
  const summary = top.data?.summary;

  const counts = [
    { label: t('all'), value: summary?.total, tone: 'default' },
    { label: t('online'), value: summary?.onlineCount, tone: 'online' },
    { label: t('depletingSoon'), value: summary?.expiringCount, tone: 'warn' },
    { label: t('depleted'), value: summary?.depletedCount, tone: 'crit' },
  ];

  return (
    <Card hoverable styles={{ body: { padding: 0 } }}>
      <div className="ov-insight-head">
        <div className="ov-kicker">{t('clients')}</div>
        <Link className="ov-insight-all" to="/clients">
          {t('pages.index.viewAll')}
        </Link>
      </div>
      <div className="ov-insight-counts" data-testid="insight-counts">
        {counts.map((c) => (
          <div key={c.label} className="ov-insight-count" data-tone={c.tone}>
            <span className="ov-insight-count-label">{c.label}</span>
            <span className="ov-tile-number">{c.value ?? '–'}</span>
          </div>
        ))}
      </div>
      <div className="ov-insight-cols">
        <InsightList
          title={t('pages.index.topUsage')}
          testId="insight-top-usage"
          clients={top.data?.items}
          isDark={isDark}
        />
        <InsightList
          title={t('pages.clients.sortExpiringSoonest')}
          testId="insight-expiring"
          clients={expiringSoon}
          isDark={isDark}
        />
      </div>
    </Card>
  );
}
