import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Popconfirm, Popover, Tag, Tooltip } from 'antd';
import { CopyOutlined, DeleteOutlined, LinkOutlined, QrcodeOutlined } from '@ant-design/icons';

import { HttpUtil } from '@/utils';
import { QrPanel } from '@/pages/inbounds/qr';

interface ShortLink {
  token: string;
  url: string;
}

interface SubShortLinkRowProps {
  subId: string;
  email: string;
  onCopy: (value: string) => void;
}

const endpoint = (subId: string) => `/panel/api/clients/subShortLink/${encodeURIComponent(subId)}`;

export default function SubShortLinkRow({ subId, email, onCopy }: SubShortLinkRowProps) {
  const { t } = useTranslation();
  const [link, setLink] = useState<ShortLink | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      const msg = await HttpUtil.get<ShortLink | null>(endpoint(subId));
      if (!cancelled) setLink(msg?.success && msg.obj ? msg.obj : null);
    })();
    return () => {
      cancelled = true;
    };
  }, [subId]);

  const create = async () => {
    setBusy(true);
    const msg = await HttpUtil.post<ShortLink>(endpoint(subId));
    setBusy(false);
    if (msg?.success && msg.obj) setLink(msg.obj);
  };

  const revoke = async () => {
    setBusy(true);
    const msg = await HttpUtil.post(`${endpoint(subId)}/revoke`);
    setBusy(false);
    if (msg?.success) setLink(null);
  };

  if (!link) {
    return (
      <div className="link-row">
        <Tag color="cyan" className="link-row-tag">
          SHORT
        </Tag>
        <span className="link-row-title">{t('pages.clients.shortLinkNone')}</span>
        <div className="link-row-actions">
          <Button size="small" icon={<LinkOutlined />} loading={busy} onClick={() => void create()}>
            {t('pages.clients.shortLinkCreate')}
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className="link-row">
      <Tag color="cyan" className="link-row-tag">
        SHORT
      </Tag>
      <a
        href={link.url}
        target="_blank"
        rel="noopener noreferrer"
        className="link-row-title link-row-title-anchor"
        title={link.url}
      >
        {link.token}
      </a>
      <div className="link-row-actions">
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
          content={<QrPanel value={link.url} remark={`${email} — SHORT`} size={220} />}
        >
          <Tooltip title={t('pages.clients.qrCode')}>
            <Button size="small" icon={<QrcodeOutlined />} aria-label={t('pages.clients.qrCode')} />
          </Tooltip>
        </Popover>
        <Popconfirm
          title={t('pages.clients.shortLinkRevokeConfirm')}
          onConfirm={() => void revoke()}
          okButtonProps={{ danger: true }}
        >
          <Tooltip title={t('pages.clients.shortLinkRevoke')}>
            <Button
              size="small"
              danger
              loading={busy}
              icon={<DeleteOutlined />}
              aria-label={t('pages.clients.shortLinkRevoke')}
            />
          </Tooltip>
        </Popconfirm>
      </div>
    </div>
  );
}
