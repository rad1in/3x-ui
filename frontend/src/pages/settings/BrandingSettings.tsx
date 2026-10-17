import { useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, ColorPicker, Input, Space } from 'antd';
import { DeleteOutlined, UploadOutlined } from '@ant-design/icons';

import type { AllSetting } from '@/models/setting';
import { SettingListItem } from '@/components/ui';
import { BRAND_LOGO_MAX_BYTES, DEFAULT_BRAND_NAME, readBrandLogo } from '@/lib/brand/brand';
import { getMessage } from '@/utils/messageBus';

const COLOR_PRESETS = [
  '#0958d9',
  '#1677ff',
  '#2f54eb',
  '#722ed1',
  '#eb2f96',
  '#d4380d',
  '#d48806',
  '#389e0d',
  '#08979c',
  '#262626',
];

interface BrandingSettingsProps {
  allSetting: AllSetting;
  updateSetting: (patch: Partial<AllSetting>) => void;
}

export default function BrandingSettings({ allSetting, updateSetting }: BrandingSettingsProps) {
  const { t } = useTranslation();
  const fileInput = useRef<HTMLInputElement>(null);

  async function onLogoPicked(file: File | undefined) {
    if (!file) return;
    const result = await readBrandLogo(file);
    if (result.ok) {
      updateSetting({ panelBrandLogo: result.dataUrl });
      return;
    }
    getMessage().error(
      result.reason === 'type'
        ? t('pages.settings.brand.logoBadType')
        : t('pages.settings.brand.logoTooLarge', { kb: BRAND_LOGO_MAX_BYTES / 1024 }),
    );
  }

  return (
    <>
      <SettingListItem
        paddings="small"
        title={t('pages.settings.brand.name')}
        description={t('pages.settings.brand.nameDesc')}
      >
        <Input
          value={allSetting.panelBrandName}
          maxLength={32}
          placeholder={DEFAULT_BRAND_NAME}
          onChange={(e) => updateSetting({ panelBrandName: e.target.value })}
        />
      </SettingListItem>

      <SettingListItem
        paddings="small"
        title={t('pages.settings.brand.color')}
        description={t('pages.settings.brand.colorDesc')}
      >
        <ColorPicker
          value={allSetting.panelBrandColor || undefined}
          disabledAlpha
          allowClear
          showText={(c) =>
            allSetting.panelBrandColor ? c.toHexString() : t('pages.settings.defaultTag')
          }
          format="hex"
          presets={[{ label: t('pages.settings.brand.color'), colors: COLOR_PRESETS }]}
          onChangeComplete={(c) => updateSetting({ panelBrandColor: c.toHexString() })}
          onClear={() => updateSetting({ panelBrandColor: '' })}
        />
      </SettingListItem>

      <SettingListItem
        paddings="small"
        title={t('pages.settings.brand.logo')}
        description={t('pages.settings.brand.logoDesc', { kb: BRAND_LOGO_MAX_BYTES / 1024 })}
      >
        <Space>
          {allSetting.panelBrandLogo && (
            <img className="brand-logo-preview" src={allSetting.panelBrandLogo} alt="" />
          )}
          <input
            ref={fileInput}
            type="file"
            accept="image/png,image/jpeg,image/webp"
            hidden
            onChange={(e) => {
              void onLogoPicked(e.target.files?.[0]);
              e.target.value = '';
            }}
          />
          <Button icon={<UploadOutlined />} onClick={() => fileInput.current?.click()}>
            {t('pages.settings.brand.logoUpload')}
          </Button>
          {allSetting.panelBrandLogo && (
            <Button
              danger
              icon={<DeleteOutlined />}
              aria-label={t('remove')}
              onClick={() => updateSetting({ panelBrandLogo: '' })}
            />
          )}
        </Space>
      </SettingListItem>
    </>
  );
}
