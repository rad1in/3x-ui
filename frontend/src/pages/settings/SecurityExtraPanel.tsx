import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Checkbox, Input, Switch } from 'antd';

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
      <SettingListItem
        paddings="small"
        title={t('pages.settings.security.extra.backupPassphrase')}
        description={t('pages.settings.security.extra.backupPassphraseDesc')}
      >
        <Input.Password
          value={cfg.backupPassphrase}
          autoComplete="new-password"
          placeholder={
            initial.hasBackupPassphrase
              ? t('pages.settings.security.extra.backupPassphraseSet')
              : ''
          }
          onChange={(e) => setCfg({ ...cfg, backupPassphrase: e.target.value })}
        />
        {initial.hasBackupPassphrase && (
          <Checkbox
            checked={cfg.clearBackupPassphrase ?? false}
            onChange={(e) => setCfg({ ...cfg, clearBackupPassphrase: e.target.checked })}
          >
            {t('pages.settings.security.extra.backupPassphraseClear')}
          </Checkbox>
        )}
      </SettingListItem>
      <SettingListItem
        paddings="small"
        title={t('pages.settings.security.extra.tgLogin')}
        description={t('pages.settings.security.extra.tgLoginDesc')}
      >
        <Switch
          checked={cfg.tgLoginEnable}
          onChange={(v) => setCfg({ ...cfg, tgLoginEnable: v })}
        />
      </SettingListItem>
      <SettingListItem
        paddings="small"
        title={t('pages.settings.security.extra.passkeyLogin')}
        description={t('pages.settings.security.extra.passkeyLoginDesc')}
      >
        <Switch
          checked={cfg.passkeyLoginEnable}
          onChange={(v) => setCfg({ ...cfg, passkeyLoginEnable: v })}
        />
      </SettingListItem>
      <SettingListItem
        paddings="small"
        title={t('pages.settings.security.extra.passkeySecondFactor')}
        description={t('pages.settings.security.extra.passkeySecondFactorDesc')}
      >
        <Switch
          checked={cfg.passkeySecondFactor}
          onChange={(v) => setCfg({ ...cfg, passkeySecondFactor: v })}
        />
      </SettingListItem>
      <SettingListItem
        paddings="small"
        title={t('pages.settings.security.extra.subSigning')}
        description={t('pages.settings.security.extra.subSigningDesc')}
      >
        <Switch
          checked={cfg.subSigningEnable}
          onChange={(v) => setCfg({ ...cfg, subSigningEnable: v })}
        />
        {initial.subSigningEnable && (
          <Checkbox
            checked={cfg.rotateSubSigningKey ?? false}
            onChange={(e) => setCfg({ ...cfg, rotateSubSigningKey: e.target.checked })}
          >
            {t('pages.settings.security.extra.subSigningRotate')}
          </Checkbox>
        )}
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
