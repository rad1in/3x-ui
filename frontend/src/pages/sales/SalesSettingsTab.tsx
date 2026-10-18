import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Form, Input, Switch } from 'antd';
import { FormProvider } from 'react-hook-form';

import { useSalesConfig } from '@/api/queries/useSales';
import { FormField, useZodForm } from '@/components/form/rhf';
import { salesConfigSchema, type SalesConfigValues } from '@/schemas/sales';

const EMPTY: SalesConfigValues = {
  publicEnabled: false,
  publicBaseUrl: '',
  card: { enabled: false, cardNumber: '', holderName: '', bankName: '', instructions: '' },
};

export default function SalesSettingsTab() {
  const { t } = useTranslation();
  const { config, save, saving } = useSalesConfig();
  const methods = useZodForm<SalesConfigValues>(salesConfigSchema, { defaultValues: EMPTY });

  useEffect(() => {
    if (config) methods.reset(config);
  }, [config, methods]);

  return (
    <FormProvider {...methods}>
      <Form layout="vertical" style={{ maxWidth: 560 }}>
        <FormField
          label={t('pages.sales.config.publicEnabled')}
          name="publicEnabled"
          valueProp="checked"
          extra={t('pages.sales.config.publicEnabledHint')}
        >
          <Switch />
        </FormField>
        <FormField
          label={t('pages.sales.config.publicBaseUrl')}
          name="publicBaseUrl"
          extra={t('pages.sales.config.publicBaseUrlHint')}
        >
          <Input placeholder="https://shop.example.com" dir="ltr" />
        </FormField>
        <h4>{t('pages.sales.config.cardTitle')}</h4>
        <FormField
          label={t('pages.sales.config.cardEnabled')}
          name="card.enabled"
          valueProp="checked"
        >
          <Switch />
        </FormField>
        <FormField label={t('pages.sales.config.cardNumber')} name="card.cardNumber">
          <Input dir="ltr" maxLength={24} inputMode="numeric" />
        </FormField>
        <FormField label={t('pages.sales.config.holderName')} name="card.holderName">
          <Input dir="auto" maxLength={80} />
        </FormField>
        <FormField label={t('pages.sales.config.bankName')} name="card.bankName">
          <Input dir="auto" maxLength={80} />
        </FormField>
        <FormField label={t('pages.sales.config.instructions')} name="card.instructions">
          <Input.TextArea dir="auto" rows={3} maxLength={500} />
        </FormField>
        <Button
          type="primary"
          loading={saving}
          onClick={methods.handleSubmit((values) => save(values))}
        >
          {t('save')}
        </Button>
      </Form>
    </FormProvider>
  );
}
