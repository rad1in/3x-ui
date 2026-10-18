import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Switch } from 'antd';

import { useSecurityConfig } from '@/api/queries/useSecurityConfig';
import { SettingListItem } from '@/components/ui';
import type { SecurityConfigView } from '@/generated/zod';

function ExtraForm({
  initial,
  saving,
  onSave,
}: {
  initial: SecurityConfigView;
  saving: boolean;
  onSave: (cfg: SecurityConfigView) => void;
}) {
  const { t } = useTranslation();
  const [cfg, setCfg] = useState(initial);
  return (
    <>
      <SettingListItem
        paddings="small"
        title={t('pages.settings.security.extra.captcha')}
        description={t('pages.settings.security.extra.captchaDesc')}
      >
        <Switch
          checked={cfg.captchaEnable}
          onChange={(v) => setCfg({ ...cfg, captchaEnable: v })}
        />
      </SettingListItem>
      <SettingListItem
        paddings="small"
        title={t('pages.settings.security.extra.newLocation')}
        description={t('pages.settings.security.extra.newLocationDesc')}
      >
        <Switch
          checked={cfg.newLocationAlert}
          onChange={(v) => setCfg({ ...cfg, newLocationAlert: v })}
        />
      </SettingListItem>
      <Button type="primary" loading={saving} onClick={() => onSave(cfg)}>
        {t('save')}
      </Button>
    </>
  );
}

// Protections that do not depend on two-factor codes; owner-only on the server.
export default function SecurityExtraPanel() {
  const { config, save, saving } = useSecurityConfig();
  if (!config) return null;
  return <ExtraForm key={JSON.stringify(config)} initial={config} saving={saving} onSave={save} />;
}
