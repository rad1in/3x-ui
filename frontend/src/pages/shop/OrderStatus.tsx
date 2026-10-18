import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Descriptions, Input, Space, Tag, Typography } from 'antd';

import type { ShopOrderView } from '@/generated/zod';
import { formatMoney } from '@/lib/money';
import { errorKey, shopApi } from '@/lib/shopApi';

interface OrderStatusProps {
  order: ShopOrderView;
  onChange: (order: ShopOrderView) => void;
}

export default function OrderStatus({ order, onChange }: OrderStatusProps) {
  const { t } = useTranslation();
  const [ref, setRef] = useState('');
  const [note, setNote] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  async function sendReceipt() {
    setBusy(true);
    setError('');
    const res = await shopApi.receipt(order.token, ref, note);
    setBusy(false);
    if (res.ok) onChange(res.data);
    else setError(errorKey(res.code));
  }

  const delivered = order.status === 'paid' && order.fulfilled;
  const awaitingTransfer =
    order.card != null && (order.status === 'pending' || order.status === 'review');

  return (
    <Space direction="vertical" size={12} style={{ width: '100%' }}>
      <Descriptions size="small" column={1}>
        <Descriptions.Item label={t('shop.plan')}>
          <span dir="auto">{order.planName}</span>
        </Descriptions.Item>
        <Descriptions.Item label={t('shop.account')}>
          <span dir="ltr">{order.clientEmail}</span>
        </Descriptions.Item>
        <Descriptions.Item label={t('shop.amount')}>
          {formatMoney(order.amount, order.currency)}
          {order.discountCode && (
            <Tag style={{ marginInlineStart: 8 }}>
              {order.discountCode}: {formatMoney(order.listAmount, order.currency)}
            </Tag>
          )}
        </Descriptions.Item>
        <Descriptions.Item label={t('shop.status')}>
          <Tag>{t(`shop.statuses.${order.status}`, { defaultValue: order.status })}</Tag>
        </Descriptions.Item>
      </Descriptions>

      {delivered && (
        <Alert
          type="success"
          showIcon
          title={t('shop.delivered')}
          description={
            order.subscriptionUrl ? (
              <Typography.Paragraph
                copyable={{ text: order.subscriptionUrl }}
                style={{ marginBottom: 0 }}
              >
                <span dir="ltr" className="shop-sub-url">
                  {order.subscriptionUrl}
                </span>
              </Typography.Paragraph>
            ) : (
              t('shop.deliveredNoLink')
            )
          }
        />
      )}
      {order.status === 'paid' && !order.fulfilled && (
        <Alert type="info" showIcon title={t('shop.paidDelivering')} />
      )}
      {order.status === 'rejected' && (
        <Alert
          type="error"
          showIcon
          title={t('shop.rejected')}
          description={order.rejectReason || undefined}
        />
      )}
      {(order.status === 'failed' || order.status === 'canceled') && (
        <Alert type="error" showIcon title={t('shop.failed')} />
      )}
      {order.status === 'pending' && order.payUrl && (
        <Button type="primary" href={order.payUrl}>
          {t('shop.payNow')}
        </Button>
      )}
      {order.status === 'review' && !order.card && (
        <Alert type="info" showIcon title={t('shop.inReview')} />
      )}

      {awaitingTransfer && order.card && (
        <Space direction="vertical" size={8} style={{ width: '100%' }}>
          <Alert
            type="info"
            showIcon
            title={t('shop.sendExactly', { amount: formatMoney(order.amount, order.currency) })}
            description={
              <Space direction="vertical" size={2}>
                <Typography.Text copyable dir="ltr" strong>
                  {order.card.cardNumber}
                </Typography.Text>
                {order.card.holderName && <span dir="auto">{order.card.holderName}</span>}
                {order.card.bankName && <span dir="auto">{order.card.bankName}</span>}
                {order.card.instructions && <span dir="auto">{order.card.instructions}</span>}
              </Space>
            }
          />
          <Input
            dir="ltr"
            value={ref}
            maxLength={64}
            placeholder={t('shop.receiptRef')}
            onChange={(e) => setRef(e.target.value)}
          />
          <Input.TextArea
            dir="auto"
            value={note}
            rows={2}
            maxLength={500}
            placeholder={t('shop.receiptNote')}
            onChange={(e) => setNote(e.target.value)}
          />
          {order.status === 'review' && (
            <Alert type="success" showIcon title={t('shop.receiptSent')} />
          )}
          {error && <Alert type="error" showIcon title={t(error)} />}
          <Button type="primary" loading={busy} disabled={!ref.trim()} onClick={sendReceipt}>
            {t('shop.sendReceipt')}
          </Button>
        </Space>
      )}
    </Space>
  );
}
