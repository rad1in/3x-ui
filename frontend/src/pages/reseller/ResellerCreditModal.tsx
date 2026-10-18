import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Divider, Input, InputNumber, Modal, Space, Statistic } from 'antd';

import { useResellerAccount, useResellerActions } from '@/api/queries/useReseller';
import ResellerLedger from './ResellerLedger';

interface ResellerCreditModalProps {
  reseller: {
    id: number;
    username: string;
    pricePerGb?: number | null;
    pricePerDay?: number | null;
  };
  onClose: () => void;
}

export default function ResellerCreditModal({ reseller, onClose }: ResellerCreditModalProps) {
  const { t } = useTranslation();
  const account = useResellerAccount(reseller.id);
  const { topup, toppingUp, setPrices, savingPrices } = useResellerActions();
  const [amount, setAmount] = useState<number | null>(null);
  const [note, setNote] = useState('');
  const [perGb, setPerGb] = useState<number | null>(reseller.pricePerGb ?? null);
  const [perDay, setPerDay] = useState<number | null>(reseller.pricePerDay ?? null);

  async function onTopup() {
    if (!amount) return;
    const msg = await topup({ id: reseller.id, amount, note });
    if (msg?.success) {
      setAmount(null);
      setNote('');
    }
  }

  return (
    <Modal
      open
      title={`${t('pages.reseller.manage')}: ${reseller.username}`}
      footer={null}
      width={680}
      mask={{ closable: false }}
      onCancel={onClose}
    >
      <Statistic title={t('pages.reseller.credit')} value={account.data?.balance ?? 0} />
      <Divider titlePlacement="start">{t('pages.reseller.topupTitle')}</Divider>
      <Space.Compact style={{ width: '100%' }}>
        <InputNumber
          value={amount}
          precision={0}
          placeholder={t('pages.reseller.amountHint')}
          onChange={setAmount}
        />
        <Input
          value={note}
          maxLength={300}
          placeholder={t('pages.reseller.note')}
          onChange={(e) => setNote(e.target.value)}
        />
        <Button type="primary" loading={toppingUp} disabled={!amount} onClick={onTopup}>
          {t('pages.reseller.topup')}
        </Button>
      </Space.Compact>
      <Divider titlePlacement="start">{t('pages.reseller.pricesTitle')}</Divider>
      <Space wrap>
        <InputNumber
          min={0}
          precision={0}
          value={perGb}
          addonBefore={t('pages.reseller.pricePerGb')}
          onChange={setPerGb}
        />
        <InputNumber
          min={0}
          precision={0}
          value={perDay}
          addonBefore={t('pages.reseller.pricePerDay')}
          onChange={setPerDay}
        />
        <Button
          loading={savingPrices}
          onClick={() => setPrices({ id: reseller.id, pricePerGb: perGb, pricePerDay: perDay })}
        >
          {t('save')}
        </Button>
      </Space>
      <Divider titlePlacement="start">{t('pages.reseller.ledger.title')}</Divider>
      <ResellerLedger entries={account.data?.entries ?? []} loading={account.isLoading} />
    </Modal>
  );
}
