import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Input, InputNumber, Select, Space, Table, Tag, Typography } from 'antd';
import type { TableColumnsType } from 'antd';

import { useWallet } from '@/api/queries/useSales';
import type { WalletEntry } from '@/generated/zod';
import { CURRENCIES, formatMoney, toMinor } from '@/lib/money';

export default function WalletsTab() {
  const { t } = useTranslation();
  const [draft, setDraft] = useState('');
  const [email, setEmail] = useState('');
  const [currency, setCurrency] = useState<string>('IRT');
  const [amount, setAmount] = useState<number | null>(null);
  const [note, setNote] = useState('');
  const { wallet, loading, adjust, adjusting } = useWallet(email);

  async function onAdjust() {
    if (!email || !amount) return;
    const msg = await adjust({ email, currency, amount: toMinor(amount, currency), note });
    if (msg?.success) {
      setAmount(null);
      setNote('');
    }
  }

  const columns: TableColumnsType<WalletEntry> = [
    {
      title: t('pages.sales.orders.created'),
      dataIndex: 'createdAt',
      render: (ms: number) => new Date(ms).toLocaleString(),
    },
    {
      title: t('pages.sales.orders.amount'),
      render: (_, e) => formatMoney(e.amount, e.currency),
    },
    {
      title: t('pages.sales.wallets.kind'),
      dataIndex: 'kind',
      render: (kind: string) => (
        <Tag>{t(`pages.sales.wallets.kinds.${kind}`, { defaultValue: kind })}</Tag>
      ),
    },
    {
      title: t('pages.sales.wallets.note'),
      dataIndex: 'note',
      render: (n: string) => <span dir="auto">{n}</span>,
    },
  ];

  return (
    <>
      <div className="sales-toolbar">
        <Input.Search
          style={{ maxWidth: 360 }}
          value={draft}
          placeholder={t('pages.sales.wallets.search')}
          enterButton
          onChange={(e) => setDraft(e.target.value)}
          onSearch={(v) => setEmail(v.trim())}
        />
      </div>
      {email && wallet && (
        <>
          <Space wrap style={{ marginBottom: 12 }}>
            {wallet.balances.length === 0 && (
              <Typography.Text type="secondary">{t('pages.sales.wallets.empty')}</Typography.Text>
            )}
            {wallet.balances.map((b) => (
              <Tag key={b.currency} color="blue">
                {formatMoney(b.amount, b.currency)}
              </Tag>
            ))}
          </Space>
          <div className="sales-toolbar">
            <Select
              value={currency}
              style={{ width: 100 }}
              options={CURRENCIES.map((c) => ({ value: c, label: c }))}
              onChange={setCurrency}
            />
            <InputNumber
              value={amount}
              placeholder={t('pages.sales.wallets.amountHint')}
              onChange={setAmount}
            />
            <Input
              value={note}
              maxLength={300}
              placeholder={t('pages.sales.wallets.note')}
              onChange={(e) => setNote(e.target.value)}
            />
            <Button type="primary" loading={adjusting} disabled={!amount} onClick={onAdjust}>
              {t('pages.sales.wallets.adjust')}
            </Button>
          </div>
          <Table<WalletEntry>
            rowKey="id"
            size="small"
            loading={loading}
            columns={columns}
            dataSource={wallet.entries}
            pagination={false}
            scroll={{ x: 560 }}
          />
        </>
      )}
    </>
  );
}
