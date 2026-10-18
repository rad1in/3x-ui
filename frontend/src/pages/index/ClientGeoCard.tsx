import { useTranslation } from 'react-i18next';
import { Card, Progress } from 'antd';

import { useClientGeo } from '@/api/queries/useClientGeo';
import { countryFlag, countryName } from '@/lib/country';

export default function ClientGeoCard() {
  const { t, i18n } = useTranslation();
  const { data } = useClientGeo();
  if (!data?.available || data.countries.length === 0) return null;

  const most = Math.max(...data.countries.map((c) => c.clients));
  return (
    <Card hoverable styles={{ body: { padding: 0 } }} data-testid="client-geo">
      <div className="ov-insight-head">
        <div className="ov-kicker">{t('pages.index.clientCountries')}</div>
      </div>
      <ul className="ov-insight-list" style={{ padding: '0 20px 12px', margin: 0 }}>
        {data.countries.map((c) => (
          <li key={c.country} className="ov-insight-row">
            <span className="ov-insight-email">
              {countryFlag(c.country)} {countryName(c.country, i18n.language)}
            </span>
            <span className="ov-insight-meta">
              {t('pages.index.clientCountriesClients', { count: c.clients })} ·{' '}
              {t('pages.index.sharedAccountsAddresses', { count: c.addresses })}
            </span>
            <Progress
              className="ov-insight-bar"
              percent={Math.round((c.clients / most) * 100)}
              size="small"
              showInfo={false}
            />
          </li>
        ))}
      </ul>
    </Card>
  );
}
