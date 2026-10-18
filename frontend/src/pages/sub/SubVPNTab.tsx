import { useTranslation } from 'react-i18next';
import { Button, Tag } from 'antd';
import { CopyOutlined, DownloadOutlined } from '@ant-design/icons';

import { vpnProfileUrl } from './subPageModel';

export interface VPNAccountRow {
  inboundId: number;
  protocol: string;
  remark: string;
  server: string;
  username: string;
  password: string;
  psk?: string;
  port?: number;
  fingerprint?: string;
}

interface SubVPNTabProps {
  subUrl: string;
  accounts: VPNAccountRow[];
  onCopy: (value: string) => void;
}

const OPENVPN_CONNECT_URL = 'https://openvpn.net/client/';
const STRONGSWAN_ANDROID_URL =
  'https://play.google.com/store/apps/details?id=org.strongswan.android';

const ROW_TAGS: Record<string, { label: string; color: string }> = {
  openvpn: { label: 'OVPN', color: 'orange' },
  ikev2: { label: 'IKEv2', color: 'blue' },
  l2tp: { label: 'L2TP', color: 'purple' },
  ssh: { label: 'SSH', color: 'cyan' },
  sstp: { label: 'SSTP', color: 'geekblue' },
  ocserv: { label: 'OpenConnect', color: 'lime' },
};

const IKEV2_DOWNLOADS = [
  { format: 'mobileconfig', labelKey: 'pages.inbounds.form.ikev2.apple' },
  { format: 'sswan', labelKey: 'pages.inbounds.form.ikev2.android' },
  { format: 'ca', labelKey: 'pages.inbounds.form.ikev2.ca' },
];

export default function SubVPNTab({ subUrl, accounts, onCopy }: SubVPNTabProps) {
  const { t } = useTranslation();
  const has = (protocol: string) => accounts.some((a) => a.protocol === protocol);
  const field = (label: string, value: string) => (
    <>
      <dt>{label}</dt>
      <dd>
        <span dir="ltr">{value}</span>
        <Button
          size="small"
          type="text"
          icon={<CopyOutlined />}
          aria-label={t('copy')}
          onClick={() => onCopy(value)}
        />
      </dd>
    </>
  );
  return (
    <div className="sub-rows">
      {has('openvpn') && (
        <p className="sub-openvpn-hint">
          {t('subscription.openvpnHint')}{' '}
          <a href={OPENVPN_CONNECT_URL} target="_blank" rel="noopener noreferrer">
            OpenVPN Connect
          </a>
        </p>
      )}
      {has('ikev2') && (
        <p className="sub-openvpn-hint">
          {t('subscription.ikev2Hint')}{' '}
          <a href={STRONGSWAN_ANDROID_URL} target="_blank" rel="noopener noreferrer">
            strongSwan
          </a>
        </p>
      )}
      {has('l2tp') && <p className="sub-openvpn-hint">{t('subscription.l2tpHint')}</p>}
      {has('ssh') && <p className="sub-openvpn-hint">{t('subscription.sshHint')}</p>}
      {has('sstp') && <p className="sub-openvpn-hint">{t('subscription.sstpHint')}</p>}
      {has('ocserv') && <p className="sub-openvpn-hint">{t('subscription.ocservHint')}</p>}
      {accounts.map((a) => (
        <div key={`${a.protocol}-${a.inboundId}-${a.username}`} className="sub-row sub-openvpn-row">
          <Tag color={ROW_TAGS[a.protocol]?.color} className="sub-row-tag">
            {ROW_TAGS[a.protocol]?.label}
          </Tag>
          <div className="sub-row-main">
            <div className="sub-row-title" dir="auto">
              {a.remark}
            </div>
            <dl className="sub-openvpn-login">
              {a.protocol !== 'openvpn' &&
                field(t('pages.inbounds.form.ikev2.serverAddress'), a.server)}
              {a.protocol === 'l2tp' && field(t('pages.inbounds.form.l2tp.psk'), a.psk ?? '')}
              {(a.protocol === 'ssh' || a.protocol === 'sstp' || a.protocol === 'ocserv') &&
                field(t('pages.inbounds.port'), String(a.port ?? ''))}
              {field(t('username'), a.username)}
              {field(t('password'), a.password)}
              {a.protocol === 'ssh' &&
                a.fingerprint &&
                field(t('pages.inbounds.form.ssh.fingerprint'), a.fingerprint)}
            </dl>
          </div>
          <div className="sub-row-actions">
            {a.protocol === 'l2tp' || a.protocol === 'ssh' ? null : a.protocol === 'sstp' ||
              a.protocol === 'ocserv' ? (
              <Button
                type="primary"
                href={vpnProfileUrl(subUrl, a.protocol, a.inboundId, 'ca')}
                icon={<DownloadOutlined />}
              >
                {t('pages.inbounds.form.sstp.ca')}
              </Button>
            ) : a.protocol === 'ikev2' ? (
              IKEV2_DOWNLOADS.map(({ format, labelKey }) => (
                <Button
                  key={format}
                  type={format === 'mobileconfig' ? 'primary' : 'default'}
                  href={vpnProfileUrl(subUrl, a.protocol, a.inboundId, format)}
                  icon={<DownloadOutlined />}
                >
                  {t(labelKey)}
                </Button>
              ))
            ) : (
              <Button
                type="primary"
                href={vpnProfileUrl(subUrl, a.protocol, a.inboundId)}
                icon={<DownloadOutlined />}
              >
                {t('pages.inbounds.form.openvpn.download')}
              </Button>
            )}
          </div>
        </div>
      ))}
    </div>
  );
}
