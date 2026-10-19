import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Segmented } from 'antd';
import {
  AndroidOutlined,
  AppleOutlined,
  DesktopOutlined,
  LaptopOutlined,
  WindowsOutlined,
} from '@ant-design/icons';

import { APP_ICONS } from './appIcons';
import { INSTALL_GUIDES } from './subPageModel';
import type { AppPlatform, SubApp } from './subPageModel';

interface SubAppsTabProps {
  apps: Record<AppPlatform, SubApp[]>;
  initialPlatform: AppPlatform;
  onOpen: (url: string) => void;
}

const PLATFORM_OPTIONS = [
  { value: 'android' as const, label: 'Android', icon: <AndroidOutlined /> },
  { value: 'ios' as const, label: 'iOS', icon: <AppleOutlined /> },
  { value: 'windows' as const, label: 'Windows', icon: <WindowsOutlined /> },
  { value: 'macos' as const, label: 'macOS', icon: <LaptopOutlined /> },
  { value: 'linux' as const, label: 'Linux', icon: <DesktopOutlined /> },
];

function AppIcon({ name }: { name: string }) {
  const icon = APP_ICONS[name];
  if (!icon) {
    return (
      <span className="sub-app-mark" aria-hidden="true">
        {name.charAt(0)}
      </span>
    );
  }
  if (icon.tinted) {
    const mask = `url("${icon.src}")`;
    return (
      <span className="sub-app-mark" aria-hidden="true">
        <span className="sub-app-glyph" style={{ maskImage: mask, WebkitMaskImage: mask }} />
      </span>
    );
  }
  return <img className="sub-app-logo" src={icon.src} alt="" width={32} height={32} />;
}

function InstallGuide({ platform }: { platform: AppPlatform }) {
  const { t } = useTranslation();
  const guide = INSTALL_GUIDES[platform];
  return (
    <section className="sub-guide" aria-label={t('subscription.guideTitle')}>
      <h3 className="sub-guide-title">{t('subscription.guideTitle')}</h3>
      <ol className="sub-guide-steps">
        <li>
          {t('subscription.guideInstall', { app: guide.app })}{' '}
          <a href={guide.url} target="_blank" rel="noopener noreferrer">
            {guide.store}
          </a>
        </li>
        <li>{t('subscription.guideImport', { app: guide.app })}</li>
        <li>{t('subscription.guideConnect')}</li>
      </ol>
    </section>
  );
}

export default function SubAppsTab({ apps, initialPlatform, onOpen }: SubAppsTabProps) {
  const { t } = useTranslation();
  const [platform, setPlatform] = useState<AppPlatform>(initialPlatform);

  return (
    <div className="sub-apps">
      <div className="sub-platforms">
        <Segmented<AppPlatform>
          value={platform}
          onChange={setPlatform}
          options={PLATFORM_OPTIONS}
        />
      </div>
      <InstallGuide platform={platform} />
      <div className="sub-app-grid">
        {apps[platform].map((app) => (
          <div key={app.name} className="sub-row">
            <AppIcon name={app.name} />
            <span className="sub-app-name">{app.name}</span>
            <Button type="primary" size="small" onClick={() => onOpen(app.url)}>
              {t('add')}
            </Button>
          </div>
        ))}
      </div>
    </div>
  );
}
