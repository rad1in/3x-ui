import { useTranslation } from 'react-i18next';
import { Card, Progress } from 'antd';

import { useProtocolTotals } from '@/api/queries/useProtocolTotals';
import { SizeFormatter } from '@/utils';

export default function ProtocolTrafficCard() {
  const { t } = useTranslation();
  const { data } = useProtocolTotals();
  if (!data?.length) return null;

  return (
    <Card hoverable styles={{ body: { padding: 0 } }} data-testid="protocol-traffic">
      <div className="ov-insight-head">
        <div className="ov-kicker">{t('pages.index.protocolTraffic')}</div>
      </div>
      <ul className="ov-insight-list" style={{ padding: '0 20px 12px', margin: 0 }}>
        {data.map((r) => (
          <li key={r.protocol} className="ov-insight-row">
            <span className="ov-insight-email">{r.protocol}</span>
            <span className="ov-insight-meta">
              {SizeFormatter.sizeFormat(r.total)} · ↑ {SizeFormatter.sizeFormat(r.up)} · ↓{' '}
              {SizeFormatter.sizeFormat(r.down)}
            </span>
            <Progress
              className="ov-insight-bar"
              percent={Math.round(r.share * 100)}
              size="small"
              showInfo
            />
          </li>
        ))}
      </ul>
    </Card>
  );
}
