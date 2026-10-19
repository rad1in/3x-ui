import { useTranslation } from 'react-i18next';
import { Tooltip, theme } from 'antd';

import type { CountryCount } from '@/generated/zod';
import { countryName } from '@/lib/country';
import world from '@/assets/world-map.json';

interface WorldMapProps {
  countries: CountryCount[];
}

// Busier countries get a stronger fill; the square root keeps one huge country
// from washing every other one out.
function intensity(clients: number, most: number): number {
  return 0.3 + 0.7 * Math.sqrt(clients / most);
}

export default function WorldMap({ countries }: WorldMapProps) {
  const { t, i18n } = useTranslation();
  const { token } = theme.useToken();
  const byCode = new Map(countries.map((c) => [c.country.toUpperCase(), c]));
  const most = Math.max(1, ...countries.map((c) => c.clients));

  return (
    <svg
      className="ov-world-map"
      viewBox={`0 0 ${world.width} ${world.height}`}
      role="group"
      aria-label={t('pages.index.clientCountries')}
      style={{ width: '100%', height: 'auto', display: 'block' }}
    >
      {world.countries.map((c) => {
        const hit = c.code ? byCode.get(c.code) : undefined;
        if (!hit) {
          return (
            <path
              key={c.name}
              d={c.d}
              fill={token.colorFillSecondary}
              stroke={token.colorBgContainer}
              strokeWidth={0.6}
            />
          );
        }
        const name = countryName(hit.country, i18n.language);
        const counts = `${t('pages.index.clientCountriesClients', { count: hit.clients })} · ${t('pages.index.sharedAccountsAddresses', { count: hit.addresses })}`;
        return (
          <Tooltip
            key={c.name}
            title={
              <>
                <strong>{name}</strong>
                <br />
                {counts}
              </>
            }
          >
            <path
              d={c.d}
              role="img"
              tabIndex={0}
              aria-label={`${name}: ${counts}`}
              data-country={hit.country.toUpperCase()}
              fill={token.colorPrimary}
              fillOpacity={intensity(hit.clients, most)}
              stroke={token.colorBgContainer}
              strokeWidth={0.6}
              style={{ outlineOffset: 2 }}
            />
          </Tooltip>
        );
      })}
    </svg>
  );
}
