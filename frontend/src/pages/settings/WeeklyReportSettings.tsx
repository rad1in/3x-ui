import { useTranslation } from 'react-i18next';
import { Select, Switch } from 'antd';
import type { AllSetting } from '@/models/setting';
import { SettingListItem } from '@/components/ui';

interface WeeklyReportSettingsProps {
  allSetting: AllSetting;
  updateSetting: (patch: Partial<AllSetting>) => void;
}

export default function WeeklyReportSettings({
  allSetting,
  updateSetting,
}: WeeklyReportSettingsProps) {
  const { t, i18n } = useTranslation();
  // 2026-01-04 is a Sunday, so day 0 = Sunday matches the backend's weekday numbering.
  const days = Array.from({ length: 7 }, (_, d) => ({
    value: d,
    label: new Date(2026, 0, 4 + d).toLocaleDateString(i18n.language, { weekday: 'long' }),
  }));
  const hours = Array.from({ length: 24 }, (_, h) => ({
    value: h,
    label: `${String(h).padStart(2, '0')}:00`,
  }));
  return (
    <>
      <SettingListItem
        paddings="small"
        title={t('pages.settings.weeklyReport')}
        description={t('pages.settings.weeklyReportDesc')}
      >
        <Switch
          checked={allSetting.weeklyReportEnable}
          onChange={(v) => updateSetting({ weeklyReportEnable: v })}
        />
      </SettingListItem>
      {allSetting.weeklyReportEnable && (
        <>
          <SettingListItem paddings="small" title={t('pages.settings.weeklyReportDay')}>
            <Select
              value={allSetting.weeklyReportDay}
              options={days}
              style={{ width: '100%' }}
              onChange={(v) => updateSetting({ weeklyReportDay: v })}
            />
          </SettingListItem>
          <SettingListItem paddings="small" title={t('pages.settings.weeklyReportHour')}>
            <Select
              value={allSetting.weeklyReportHour}
              options={hours}
              style={{ width: '100%' }}
              onChange={(v) => updateSetting({ weeklyReportHour: v })}
            />
          </SettingListItem>
        </>
      )}
    </>
  );
}
