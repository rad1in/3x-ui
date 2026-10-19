import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Checkbox, Form, Input, InputNumber, Switch, Typography } from 'antd';
import { FormProvider, useWatch } from 'react-hook-form';

import { useSalesConfig } from '@/api/queries/useSales';
import { FormField, useZodForm } from '@/components/form/rhf';
import { salesConfigSchema, type SalesConfigValues } from '@/schemas/sales';

const EMPTY: SalesConfigValues = {
  publicEnabled: false,
  publicBaseUrl: '',
  card: { enabled: false, cardNumber: '', holderName: '', bankName: '', instructions: '' },
  zarinpal: { enabled: false, merchantId: '', sandbox: false },
  idpay: { enabled: false, apiKey: '', sandbox: false },
  crypto: { enabled: false, apiKey: '', ipnSecret: '', sandbox: false },
  walletEnabled: false,
  referral: { enabled: false, bonusPercent: 10 },
  clearSecrets: [],
};

function shopLink(baseUrl: string): string {
  const origin = baseUrl.trim().replace(/\/+$/, '') || window.location.origin;
  return `${origin}/shop/`;
}

const SECRET_MERCHANT = 'zarinpal.merchantId';
const SECRET_IDPAY = 'idpay.apiKey';
const SECRET_CRYPTO_KEY = 'crypto.apiKey';
const SECRET_CRYPTO_IPN = 'crypto.ipnSecret';

export default function SalesSettingsTab() {
  const { t } = useTranslation();
  const { config, save, saving } = useSalesConfig();
  const methods = useZodForm<SalesConfigValues>(salesConfigSchema, { defaultValues: EMPTY });
  const clearSecrets = useWatch({ control: methods.control, name: 'clearSecrets' }) ?? [];

  const publicBaseUrl = useWatch({ control: methods.control, name: 'publicBaseUrl' }) ?? '';

  useEffect(() => {
    if (config) methods.reset({ ...config, clearSecrets: [] });
  }, [config, methods]);

  const toggleClear = (name: string, on: boolean) =>
    methods.setValue(
      'clearSecrets',
      on ? [...clearSecrets, name] : clearSecrets.filter((n) => n !== name),
    );

  const secretField = (name: string, isSet: boolean | undefined) =>
    isSet ? (
      <Checkbox
        checked={clearSecrets.includes(name)}
        onChange={(e) => toggleClear(name, e.target.checked)}
      >
        {t('pages.sales.config.removeSecret')}
      </Checkbox>
    ) : null;

  return (
    <FormProvider {...methods}>
      <Form layout="vertical" style={{ maxWidth: 560 }}>
        <FormField
          label={t('pages.sales.config.publicEnabled')}
          name="publicEnabled"
          valueProp="checked"
          extra={
            <>
              {t('pages.sales.config.publicEnabledHint')}
              <br />
              {t('pages.sales.config.shopLink')}{' '}
              <Typography.Text copyable dir="ltr">
                {shopLink(publicBaseUrl)}
              </Typography.Text>
            </>
          }
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

        <h4>{t('pages.sales.config.walletTitle')}</h4>
        <FormField
          label={t('pages.sales.config.walletEnabled')}
          name="walletEnabled"
          valueProp="checked"
          extra={t('pages.sales.config.walletHint')}
        >
          <Switch />
        </FormField>
        <FormField
          label={t('pages.sales.config.referralEnabled')}
          name="referral.enabled"
          valueProp="checked"
          extra={t('pages.sales.config.referralHint')}
        >
          <Switch />
        </FormField>
        <FormField label={t('pages.sales.config.bonusPercent')} name="referral.bonusPercent">
          <InputNumber min={0} max={100} precision={0} style={{ width: '100%' }} />
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

        <h4>{t('pages.sales.config.zarinpalTitle')}</h4>
        <FormField
          label={t('pages.sales.config.gatewayEnabled')}
          name="zarinpal.enabled"
          valueProp="checked"
          extra={t('pages.sales.config.gatewayHint')}
        >
          <Switch />
        </FormField>
        <FormField
          label={t('pages.sales.config.merchantId')}
          name="zarinpal.merchantId"
          extra={secretField(SECRET_MERCHANT, config?.hasZarinpalMerchantId)}
        >
          <Input.Password
            dir="ltr"
            autoComplete="off"
            placeholder={config?.hasZarinpalMerchantId ? t('pages.sales.config.secretKept') : ''}
          />
        </FormField>
        <FormField
          label={t('pages.sales.config.sandbox')}
          name="zarinpal.sandbox"
          valueProp="checked"
        >
          <Switch />
        </FormField>

        <h4>{t('pages.sales.config.idpayTitle')}</h4>
        <FormField
          label={t('pages.sales.config.gatewayEnabled')}
          name="idpay.enabled"
          valueProp="checked"
          extra={t('pages.sales.config.gatewayHint')}
        >
          <Switch />
        </FormField>
        <FormField
          label={t('pages.sales.config.apiKey')}
          name="idpay.apiKey"
          extra={secretField(SECRET_IDPAY, config?.hasIdpayApiKey)}
        >
          <Input.Password
            dir="ltr"
            autoComplete="off"
            placeholder={config?.hasIdpayApiKey ? t('pages.sales.config.secretKept') : ''}
          />
        </FormField>
        <FormField label={t('pages.sales.config.sandbox')} name="idpay.sandbox" valueProp="checked">
          <Switch />
        </FormField>

        <h4>{t('pages.sales.config.cryptoTitle')}</h4>
        <FormField
          label={t('pages.sales.config.gatewayEnabled')}
          name="crypto.enabled"
          valueProp="checked"
          extra={t('pages.sales.config.cryptoHint')}
        >
          <Switch />
        </FormField>
        <FormField
          label={t('pages.sales.config.apiKey')}
          name="crypto.apiKey"
          extra={secretField(SECRET_CRYPTO_KEY, config?.hasCryptoApiKey)}
        >
          <Input.Password
            dir="ltr"
            autoComplete="off"
            placeholder={config?.hasCryptoApiKey ? t('pages.sales.config.secretKept') : ''}
          />
        </FormField>
        <FormField
          label={t('pages.sales.config.ipnSecret')}
          name="crypto.ipnSecret"
          extra={secretField(SECRET_CRYPTO_IPN, config?.hasCryptoIpnSecret)}
        >
          <Input.Password
            dir="ltr"
            autoComplete="off"
            placeholder={config?.hasCryptoIpnSecret ? t('pages.sales.config.secretKept') : ''}
          />
        </FormField>
        <FormField
          label={t('pages.sales.config.sandbox')}
          name="crypto.sandbox"
          valueProp="checked"
        >
          <Switch />
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
