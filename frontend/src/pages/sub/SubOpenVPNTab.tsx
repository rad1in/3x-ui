import { useTranslation } from 'react-i18next';
import { Button, Tag } from 'antd';
import { CopyOutlined, DownloadOutlined } from '@ant-design/icons';

import { openvpnProfileUrl } from './subPageModel';

export interface OpenVPNProfileRow {
  inboundId: number;
  remark: string;
  username: string;
  password: string;
}

interface SubOpenVPNTabProps {
  subUrl: string;
  profiles: OpenVPNProfileRow[];
  onCopy: (value: string) => void;
}

const OPENVPN_CONNECT_URL = 'https://openvpn.net/client/';

export default function SubOpenVPNTab({ subUrl, profiles, onCopy }: SubOpenVPNTabProps) {
  const { t } = useTranslation();
  return (
    <div className="sub-rows">
      <p className="sub-openvpn-hint">
        {t('subscription.openvpnHint')}{' '}
        <a href={OPENVPN_CONNECT_URL} target="_blank" rel="noopener noreferrer">
          OpenVPN Connect
        </a>
      </p>
      {profiles.map((p) => (
        <div key={`${p.inboundId}-${p.username}`} className="sub-row sub-openvpn-row">
          <Tag color="orange" className="sub-row-tag">
            OVPN
          </Tag>
          <div className="sub-row-main">
            <div className="sub-row-title" dir="auto">
              {p.remark}
            </div>
            <dl className="sub-openvpn-login">
              <dt>{t('username')}</dt>
              <dd>
                <span dir="ltr">{p.username}</span>
                <Button
                  size="small"
                  type="text"
                  icon={<CopyOutlined />}
                  aria-label={t('copy')}
                  onClick={() => onCopy(p.username)}
                />
              </dd>
              <dt>{t('password')}</dt>
              <dd>
                <span dir="ltr">{p.password}</span>
                <Button
                  size="small"
                  type="text"
                  icon={<CopyOutlined />}
                  aria-label={t('copy')}
                  onClick={() => onCopy(p.password)}
                />
              </dd>
            </dl>
          </div>
          <div className="sub-row-actions">
            <Button
              type="primary"
              href={openvpnProfileUrl(subUrl, p.inboundId)}
              icon={<DownloadOutlined />}
            >
              {t('pages.inbounds.form.openvpn.download')}
            </Button>
          </div>
        </div>
      ))}
    </div>
  );
}
