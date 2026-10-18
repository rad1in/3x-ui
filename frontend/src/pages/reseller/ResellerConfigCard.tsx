import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Card, InputNumber, Space } from 'antd';
import { TagsOutlined } from '@ant-design/icons';

import { useResellerConfig } from '@/api/queries/useReseller';
import type { ResellerConfig } from '@/generated/zod';

function ConfigForm({
  initial,
  saving,
  onSave,
}: {
  initial: ResellerConfig;
  saving: boolean;
  onSave: (cfg: ResellerConfig) => void;
}) {
  const { t } = useTranslation();
  const [cfg, setCfg] = useState(initial);
  return (
    <Space wrap align="end">
      <InputNumber
        min={0}
        precision={0}
        value={cfg.pricePerGb}
        addonBefore={t('pages.reseller.pricePerGb')}
        onChange={(v) => setCfg({ ...cfg, pricePerGb: v ?? 0 })}
      />
      <InputNumber
        min={0}
        precision={0}
        value={cfg.pricePerDay}
        addonBefore={t('pages.reseller.pricePerDay')}
        onChange={(v) => setCfg({ ...cfg, pricePerDay: v ?? 0 })}
      />
      <InputNumber
        min={0}
        precision={0}
        value={cfg.lowCredit}
        addonBefore={t('pages.reseller.lowCredit')}
        onChange={(v) => setCfg({ ...cfg, lowCredit: v ?? 0 })}
      />
      <Button type="primary" loading={saving} onClick={() => onSave(cfg)}>
        {t('save')}
      </Button>
    </Space>
  );
}

// The panel-wide price list; a reseller without its own prices is billed by it.
export default function ResellerConfigCard() {
  const { t } = useTranslation();
  const { config, save, saving } = useResellerConfig(true);
  return (
    <Card
      size="small"
      className="reseller-card"
      title={
        <span className="reseller-title">
          <TagsOutlined /> {t('pages.reseller.wholesale')}
        </span>
      }
    >
      <p className="reseller-hint" dir="auto">
        {t('pages.reseller.wholesaleHint')}
      </p>
      {config && (
        <ConfigForm key={JSON.stringify(config)} initial={config} saving={saving} onSave={save} />
      )}
    </Card>
  );
}
