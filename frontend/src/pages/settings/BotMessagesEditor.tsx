import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Input, Select, Typography } from 'antd';
import { HttpUtil, LanguageManager } from '@/utils';
import type { AllSetting } from '@/models/setting';
import { SettingListItem } from '@/components/ui';

interface BotMessagesEditorProps {
  allSetting: AllSetting;
  updateSetting: (patch: Partial<AllSetting>) => void;
}

type Overrides = Record<string, Record<string, string>>;

function parseOverrides(raw: string): Overrides {
  try {
    const v: unknown = JSON.parse(raw || '{}');
    return v && typeof v === 'object' ? (v as Overrides) : {};
  } catch {
    return {};
  }
}

export default function BotMessagesEditor({ allSetting, updateSetting }: BotMessagesEditorProps) {
  const { t } = useTranslation();
  const [lang, setLang] = useState(allSetting.tgLang || 'en-US');
  const [defaults, setDefaults] = useState<Record<string, string>>({});
  const overrides = useMemo(
    () => parseOverrides(allSetting.tgBotMessageOverrides),
    [allSetting.tgBotMessageOverrides],
  );

  useEffect(() => {
    let live = true;
    HttpUtil.post('/panel/api/setting/botMessages', { lang }).then((res) => {
      const r = res as { success?: boolean; obj?: Record<string, string> };
      if (live && r.success && r.obj) setDefaults(r.obj);
    });
    return () => {
      live = false;
    };
  }, [lang]);

  function setText(key: string, text: string) {
    const next: Overrides = { ...overrides, [lang]: { ...overrides[lang], [key]: text } };
    if (text.trim() === '') delete next[lang][key];
    if (Object.keys(next[lang]).length === 0) delete next[lang];
    updateSetting({
      tgBotMessageOverrides: Object.keys(next).length === 0 ? '' : JSON.stringify(next),
    });
  }

  return (
    <>
      <SettingListItem
        paddings="small"
        title={t('pages.settings.botMessages')}
        description={t('pages.settings.botMessagesDesc')}
      >
        <Select
          value={lang}
          onChange={setLang}
          style={{ width: '100%' }}
          options={LanguageManager.supportedLanguages.map((l: { value: string; name: string }) => ({
            value: l.value,
            label: l.name,
          }))}
        />
      </SettingListItem>
      {Object.keys(defaults)
        .sort()
        .map((key) => (
          <SettingListItem
            key={key}
            paddings="small"
            title={<Typography.Text code>{key}</Typography.Text>}
          >
            <Input.TextArea
              value={overrides[lang]?.[key] ?? ''}
              placeholder={defaults[key]}
              autoSize={{ minRows: 2, maxRows: 8 }}
              maxLength={4000}
              onChange={(e) => setText(key, e.target.value)}
              aria-label={key}
            />
          </SettingListItem>
        ))}
    </>
  );
}
