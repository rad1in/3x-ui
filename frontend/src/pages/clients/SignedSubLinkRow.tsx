import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Popover, Select, Tag, Tooltip } from 'antd';
import { CopyOutlined, LinkOutlined, QrcodeOutlined } from '@ant-design/icons';

import { SignedSubLinkSchema, type SignedSubLink } from '@/generated/zod';
import { QrPanel } from '@/pages/inbounds/qr';
import { HttpUtil } from '@/utils';
import { parseMsg } from '@/utils/zodValidate';

const HOUR = 3600;
const TTLS = [
  { value: HOUR, key: 'signedTtl1h' },
  { value: 24 * HOUR, key: 'signedTtl1d' },
  { value: 7 * 24 * HOUR, key: 'signedTtl7d' },
  { value: 30 * 24 * HOUR, key: 'signedTtl30d' },
  { value: 0, key: 'signedTtlNever' },
] as const;

interface SignedSubLinkRowProps {
  subId: string;
  email: string;
  onCopy: (value: string) => void;
}

// A signed link is what the subscription server accepts once signing is enforced.
export default function SignedSubLinkRow({ subId, email, onCopy }: SignedSubLinkRowProps) {
  const { t } = useTranslation();
  const [ttl, setTtl] = useState<number>(7 * 24 * HOUR);
  const [link, setLink] = useState<SignedSubLink | null>(null);
  const [busy, setBusy] = useState(false);

  const create = async () => {
    setBusy(true);
    const msg = await HttpUtil.post(
      `/panel/api/clients/signedSubLink/${encodeURIComponent(subId)}`,
      { ttlSeconds: ttl },
      { headers: { 'Content-Type': 'application/json' } },
    );
    setBusy(false);
    if (msg?.success)
      setLink(parseMsg(msg, SignedSubLinkSchema, 'clients/signedSubLink').obj ?? null);
  };

  return (
    <div className="link-row">
      <Tag color="gold" className="link-row-tag">
        SIGNED
      </Tag>
      {link ? (
        <a
          href={link.url}
          target="_blank"
          rel="noopener noreferrer"
          className="link-row-title link-row-title-anchor"
          title={link.url}
        >
          {link.expiresAt > 0
            ? t('pages.clients.signedExpires', { date: new Date(link.expiresAt).toLocaleString() })
            : t('pages.clients.signedTtlNever')}
        </a>
      ) : (
        <span className="link-row-title">{t('pages.clients.signedLinkNone')}</span>
      )}
      <div className="link-row-actions">
        <Select
          size="small"
          value={ttl}
          style={{ width: 110 }}
          options={TTLS.map((o) => ({ value: o.value, label: t(`pages.clients.${o.key}`) }))}
          onChange={setTtl}
        />
        <Button size="small" icon={<LinkOutlined />} loading={busy} onClick={() => void create()}>
          {t('pages.clients.signedLinkCreate')}
        </Button>
        {link && (
          <>
            <Tooltip title={t('copy')}>
              <Button
                size="small"
                icon={<CopyOutlined />}
                aria-label={t('copy')}
                onClick={() => onCopy(link.url)}
              />
            </Tooltip>
            <Popover
              trigger="click"
              placement="left"
              destroyOnHidden
              content={<QrPanel value={link.url} remark={`${email} — SIGNED`} size={220} />}
            >
              <Tooltip title={t('pages.clients.qrCode')}>
                <Button
                  size="small"
                  icon={<QrcodeOutlined />}
                  aria-label={t('pages.clients.qrCode')}
                />
              </Tooltip>
            </Popover>
          </>
        )}
      </div>
    </div>
  );
}
