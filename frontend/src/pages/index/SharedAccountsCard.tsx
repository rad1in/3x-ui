import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';
import { Card } from 'antd';

import { useSharedAccounts } from '@/api/queries/useSharedAccounts';
import { IntlUtil } from '@/utils';

export default function SharedAccountsCard() {
  const { t } = useTranslation();
  const { data } = useSharedAccounts();
  if (!data?.length) return null;

  return (
    <Card hoverable styles={{ body: { padding: 0 } }} data-testid="shared-accounts">
      <div className="ov-insight-head">
        <div className="ov-kicker">{t('pages.index.sharedAccounts')}</div>
      </div>
      <ul className="ov-insight-list" style={{ padding: '0 20px 12px', margin: 0 }}>
        {data.map((f) => (
          <li key={f.email} className="ov-insight-row">
            <Link
              className="ov-insight-email"
              to={`/clients?search=${encodeURIComponent(f.email)}`}
            >
              {f.email}
            </Link>
            <span className="ov-insight-meta">
              {t('pages.index.sharedAccountsAddresses', { count: f.ipCount })}
              {f.countryCount > 0 &&
                ` · ${t('pages.index.sharedAccountsCountries', { count: f.countryCount })} (${f.countries})`}
              {` · ${IntlUtil.formatRelativeTime(f.lastSeen)}`}
            </span>
          </li>
        ))}
      </ul>
    </Card>
  );
}
