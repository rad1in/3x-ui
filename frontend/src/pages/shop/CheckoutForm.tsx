import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Input, Radio, Select, Space, Typography } from 'antd';

import type { ShopOrderView, ShopPlan } from '@/generated/zod';
import { formatMoney } from '@/lib/money';
import { errorKey, shopApi } from '@/lib/shopApi';

interface CheckoutFormProps {
  plan: ShopPlan;
  methods: string[];
  onOrder: (view: ShopOrderView) => void;
}

export default function CheckoutForm({ plan, methods, onOrder }: CheckoutFormProps) {
  const { t } = useTranslation();
  const [kind, setKind] = useState<'new' | 'renew'>('new');
  const [email, setEmail] = useState('');
  const [contact, setContact] = useState('');
  const [discountCode, setDiscountCode] = useState('');
  const [referralCode, setReferralCode] = useState('');
  const [proof, setProof] = useState('');
  const [gateway, setGateway] = useState(methods[0] ?? '');
  const [final, setFinal] = useState<number | null>(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  const wallet = gateway === 'wallet';
  const methodOptions = methods
    .filter((m) => m !== 'wallet' || kind === 'renew')
    .map((m) => ({ value: m, label: t(`pages.sales.gateways.${m}`, { defaultValue: m }) }));

  async function applyCode() {
    setError('');
    setFinal(null);
    if (!discountCode.trim()) return;
    const res = await shopApi.quote({ planId: plan.id, discountCode, email });
    if (res.ok) setFinal(res.data.final);
    else setError(errorKey(res.code));
  }

  async function submit() {
    setBusy(true);
    setError('');
    const res = await shopApi.createOrder({
      planId: plan.id,
      kind,
      email,
      gateway,
      contact,
      discountCode,
      referralCode: kind === 'new' ? referralCode : '',
      proof: wallet ? proof : '',
    });
    setBusy(false);
    if (!res.ok) {
      setError(errorKey(res.code));
      return;
    }
    onOrder(res.data);
    if (res.data.payUrl) window.location.assign(res.data.payUrl);
  }

  if (methods.length === 0) return <Alert type="warning" showIcon title={t('shop.noMethods')} />;

  return (
    <Space direction="vertical" size={12} style={{ width: '100%' }}>
      <Typography.Text strong dir="auto">
        {plan.name}: {formatMoney(final ?? plan.price, plan.currency)}
      </Typography.Text>
      <Radio.Group
        value={kind}
        onChange={(e) => {
          setKind(e.target.value);
          if (e.target.value === 'new' && gateway === 'wallet')
            setGateway(methods.find((m) => m !== 'wallet') ?? '');
        }}
      >
        <Radio.Button value="new">{t('shop.newAccount')}</Radio.Button>
        <Radio.Button value="renew">{t('shop.renewAccount')}</Radio.Button>
      </Radio.Group>
      <Input
        dir="ltr"
        value={email}
        maxLength={64}
        placeholder={kind === 'new' ? t('shop.chooseUsername') : t('shop.yourUsername')}
        onChange={(e) => setEmail(e.target.value)}
      />
      <Input
        dir="auto"
        value={contact}
        maxLength={100}
        placeholder={t('shop.contact')}
        onChange={(e) => setContact(e.target.value)}
      />
      <Input.Search
        dir="ltr"
        value={discountCode}
        placeholder={t('shop.discountCode')}
        enterButton={t('shop.apply')}
        onChange={(e) => {
          setDiscountCode(e.target.value);
          setFinal(null);
        }}
        onSearch={applyCode}
      />
      {kind === 'new' && (
        <Input
          dir="ltr"
          value={referralCode}
          maxLength={16}
          placeholder={t('shop.referralCode')}
          onChange={(e) => setReferralCode(e.target.value)}
        />
      )}
      <Select
        value={gateway}
        options={methodOptions}
        style={{ width: '100%' }}
        onChange={setGateway}
        aria-label={t('shop.payWith')}
      />
      {wallet && (
        <Input.Password
          dir="ltr"
          value={proof}
          placeholder={t('shop.subscriptionId')}
          onChange={(e) => setProof(e.target.value)}
        />
      )}
      {error && <Alert type="error" showIcon title={t(error)} />}
      <Button
        type="primary"
        block
        loading={busy}
        disabled={!email.trim() || !gateway}
        onClick={submit}
      >
        {t('shop.buy')}
      </Button>
    </Space>
  );
}
