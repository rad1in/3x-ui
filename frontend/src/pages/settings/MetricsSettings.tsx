import { useTranslation } from 'react-i18next';
import { Button, Input, Space, Switch, Typography } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';

import type { AllSetting } from '@/models/setting';
import { SettingListItem } from '@/components/ui';

interface MetricsSettingsProps {
  allSetting: AllSetting;
  updateSetting: (patch: Partial<AllSetting>) => void;
}

function randomToken(): string {
  const bytes = new Uint8Array(24);
  crypto.getRandomValues(bytes);
  return Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
}

export default function MetricsSettings({ allSetting, updateSetting }: MetricsSettingsProps) {
  const { t } = useTranslation();
  const base = (window.X_UI_BASE_PATH || '/').replace(/\/?$/, '/');
  return (
    <>
      <SettingListItem
        paddings="small"
        title={t('pages.settings.metrics.enable')}
        description={t('pages.settings.metrics.enableDesc')}
      >
        <Switch
          checked={allSetting.metricsEnable}
          onChange={(on) =>
            updateSetting({
              metricsEnable: on,
              ...(on && !allSetting.metricsToken ? { metricsToken: randomToken() } : {}),
            })
          }
        />
      </SettingListItem>
      {allSetting.metricsEnable && (
        <>
          <SettingListItem
            paddings="small"
            title={t('pages.settings.metrics.token')}
            description={t('pages.settings.metrics.tokenDesc')}
          >
            <Space.Compact style={{ width: '100%' }}>
              <Input
                value={allSetting.metricsToken}
                style={{ direction: 'ltr' }}
                aria-label={t('pages.settings.metrics.token')}
                onChange={(e) => updateSetting({ metricsToken: e.target.value.trim() })}
              />
              <Button
                icon={<ReloadOutlined />}
                aria-label={t('pages.settings.metrics.regenerate')}
                onClick={() => updateSetting({ metricsToken: randomToken() })}
              />
            </Space.Compact>
          </SettingListItem>
          <Typography.Paragraph type="secondary" style={{ marginTop: 8 }}>
            {t('pages.settings.metrics.scrapeHint')}
            <br />
            <code style={{ direction: 'ltr', unicodeBidi: 'isolate' }}>
              {`${window.location.origin}${base}metrics`}
            </code>
          </Typography.Paragraph>
        </>
      )}
    </>
  );
}
