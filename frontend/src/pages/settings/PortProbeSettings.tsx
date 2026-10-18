import { useTranslation } from 'react-i18next';
import { Input, InputNumber, Switch } from 'antd';
import type { AllSetting } from '@/models/setting';
import { SettingListItem } from '@/components/ui';
import { onNumber } from '@/utils/onNumber';

interface PortProbeSettingsProps {
  allSetting: AllSetting;
  updateSetting: (patch: Partial<AllSetting>) => void;
}

export default function PortProbeSettings({ allSetting, updateSetting }: PortProbeSettingsProps) {
  const { t } = useTranslation();
  return (
    <>
      <SettingListItem
        paddings="small"
        title={t('pages.settings.portProbe')}
        description={t('pages.settings.portProbeDesc')}
      >
        <Switch
          checked={allSetting.portProbeEnable}
          onChange={(v) => updateSetting({ portProbeEnable: v })}
        />
      </SettingListItem>
      {allSetting.portProbeEnable && (
        <>
          <SettingListItem
            paddings="small"
            title={t('pages.settings.portProbeHost')}
            description={t('pages.settings.portProbeHostDesc')}
          >
            <Input
              value={allSetting.portProbeHost}
              style={{ direction: 'ltr' }}
              onChange={(e) => updateSetting({ portProbeHost: e.target.value })}
            />
          </SettingListItem>
          <SettingListItem
            paddings="small"
            title={t('pages.settings.portProbeUrl')}
            description={t('pages.settings.portProbeUrlDesc')}
          >
            <Input
              value={allSetting.portProbeUrl}
              placeholder="https://checker.example/check?host={host}&port={port}"
              style={{ direction: 'ltr' }}
              onChange={(e) => updateSetting({ portProbeUrl: e.target.value })}
            />
          </SettingListItem>
          {allSetting.portProbeUrl.trim() !== '' && (
            <SettingListItem
              paddings="small"
              title={t('pages.settings.portProbeExpect')}
              description={t('pages.settings.portProbeExpectDesc')}
            >
              <Input
                value={allSetting.portProbeExpect}
                style={{ direction: 'ltr' }}
                onChange={(e) => updateSetting({ portProbeExpect: e.target.value })}
              />
            </SettingListItem>
          )}
          <SettingListItem
            paddings="small"
            title={t('pages.settings.portProbeFailures')}
            description={t('pages.settings.portProbeFailuresDesc')}
          >
            <InputNumber
              min={1}
              max={20}
              value={allSetting.portProbeFailures}
              onChange={onNumber((v) => updateSetting({ portProbeFailures: v }))}
            />
          </SettingListItem>
        </>
      )}
    </>
  );
}
