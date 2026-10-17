import { useTranslation } from 'react-i18next';
import { Button, QRCode, Tag } from 'antd';
import { CopyOutlined, DownloadOutlined, ImportOutlined } from '@ant-design/icons';

import SubQrButton from './SubQrButton';

interface SubLinksTabProps {
  subUrl: string;
  subJsonUrl: string;
  subClashUrl: string;
  subSingboxUrl?: string;
  outlineKeys?: { inboundId: number; remark: string; url: string }[];
  onCopy: (value: string) => void;
}

const appendRawView = (url: string) => `${url}${url.includes('?') ? '&' : '?'}view=raw`;

// sing-box's own deep link; the app fetches and refreshes the profile itself.
const singboxImportUrl = (url: string) =>
  `sing-box://import-remote-profile?url=${encodeURIComponent(url)}#${encodeURIComponent(new URL(url).host)}`;

export default function SubLinksTab({
  subUrl,
  subJsonUrl,
  subClashUrl,
  subSingboxUrl = '',
  outlineKeys = [],
  onCopy,
}: SubLinksTabProps) {
  const { t } = useTranslation();
  const subLabel = t('pages.settings.subSettings');
  const rows = [
    { kind: 'SUB', color: 'green', url: subUrl, title: subLabel, downloadable: false },
    {
      kind: 'JSON',
      color: 'purple',
      url: subJsonUrl,
      title: `${subLabel} JSON`,
      downloadable: true,
    },
    { kind: 'CLASH', color: 'gold', url: subClashUrl, title: 'Clash / Mihomo', downloadable: true },
    {
      kind: 'SING-BOX',
      color: 'blue',
      url: subSingboxUrl,
      title: 'sing-box',
      downloadable: false,
      importUrl: subSingboxUrl && singboxImportUrl(subSingboxUrl),
    },
    ...outlineKeys.map((key) => ({
      kind: `OUTLINE-${key.inboundId}`,
      tag: 'OUTLINE',
      color: 'cyan',
      url: key.url,
      title: `Outline · ${key.remark}`,
      downloadable: false,
      importUrl: key.url,
    })),
  ].filter((row) => row.url);

  return (
    <div className="sub-rows">
      {rows.map((row) => (
        <div key={row.kind} className="sub-row">
          <Tag color={row.color} className="sub-row-tag">
            {'tag' in row ? row.tag : row.kind}
          </Tag>
          <div className="sub-row-main">
            <a href={row.url} target="_blank" rel="noopener noreferrer" className="sub-row-title">
              {row.title}
            </a>
            <div className="sub-row-url" dir="ltr" title={row.url}>
              {row.url}
            </div>
          </div>
          <div className="sub-row-actions">
            {row.downloadable && (
              <Button
                href={appendRawView(row.url)}
                target="_blank"
                rel="noopener noreferrer"
                icon={<DownloadOutlined />}
                aria-label={t('download')}
                title={t('download')}
              />
            )}
            {'importUrl' in row && row.importUrl && (
              <Button
                href={row.importUrl}
                icon={<ImportOutlined />}
                aria-label={t('subscription.importToApp')}
                title={t('subscription.importToApp')}
              />
            )}
            <Button
              icon={<CopyOutlined />}
              onClick={() => onCopy(row.url)}
              aria-label={t('copy')}
              title={t('copy')}
            />
            <SubQrButton value={row.url} label={row.title} onCopy={onCopy} />
          </div>
        </div>
      ))}
      {outlineKeys.length > 0 && <p className="sub-muted">{t('subscription.outlineHint')}</p>}
      {subUrl && (
        <div className="sub-qr-card">
          <div className="sub-qr-code">
            <QRCode
              value={subUrl}
              size={112}
              type="svg"
              bordered={false}
              color="#000000"
              bgColor="#ffffff"
            />
          </div>
          <div>
            <div className="sub-qr-title">{t('subscription.scanTitle')}</div>
            <div className="sub-muted">{t('subscription.scanHint')}</div>
          </div>
        </div>
      )}
    </div>
  );
}
