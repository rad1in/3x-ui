import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Card, ConfigProvider, Layout, Spin, Typography } from 'antd';

import type { ShopCatalog, ShopOrderView } from '@/generated/zod';
import { shopApi } from '@/lib/shopApi';
import { useTheme } from '@/hooks/useTheme';
import CheckoutForm from './CheckoutForm';
import OrderStatus from './OrderStatus';
import PlanList from './PlanList';
import './ShopPage.css';

const POLL_MS = 5000;

function tokenFromUrl(): string {
  return new URLSearchParams(window.location.search).get('order') ?? '';
}

function isSettling(order: ShopOrderView): boolean {
  if (order.status === 'pending' || order.status === 'review') return true;
  return order.status === 'paid' && !order.fulfilled;
}

export default function ShopPage() {
  const { t } = useTranslation();
  const { antdThemeConfig } = useTheme();
  const [catalog, setCatalog] = useState<ShopCatalog | null>(null);
  const [loadFailed, setLoadFailed] = useState(false);
  const [planId, setPlanId] = useState<number | null>(null);
  const [order, setOrder] = useState<ShopOrderView | null>(null);
  const [booting, setBooting] = useState(true);

  const show = useCallback((view: ShopOrderView) => {
    setOrder(view);
    const url = new URL(window.location.href);
    url.searchParams.set('order', view.token);
    window.history.replaceState(null, '', url);
  }, []);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      const token = tokenFromUrl();
      const [plans, existing] = await Promise.all([
        shopApi.catalog(),
        token ? shopApi.order(token) : Promise.resolve(null),
      ]);
      if (cancelled) return;
      if (plans.ok) setCatalog(plans.data);
      else setLoadFailed(true);
      if (existing?.ok) setOrder(existing.data);
      setBooting(false);
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  const token = order?.token;
  const settling = order ? isSettling(order) : false;
  useEffect(() => {
    if (!token || !settling) return undefined;
    const timer = window.setInterval(() => {
      void shopApi.order(token).then((res) => {
        if (res.ok) setOrder(res.data);
      });
    }, POLL_MS);
    return () => window.clearInterval(timer);
  }, [token, settling]);

  const plan = catalog?.plans.find((p) => p.id === planId) ?? null;

  return (
    <ConfigProvider theme={antdThemeConfig}>
      <Layout className="shop-page">
        <Layout.Content className="shop-content">
          <Typography.Title level={3} className="shop-title">
            {t('shop.title')}
          </Typography.Title>
          {booting && <Spin />}
          {!booting && loadFailed && (
            <Alert type="error" showIcon title={t('shop.errors.network')} />
          )}
          {!booting && order && (
            <Card size="small">
              <OrderStatus order={order} onChange={setOrder} />
            </Card>
          )}
          {!booting && !order && catalog && (
            <>
              <PlanList plans={catalog.plans} selected={planId} onSelect={setPlanId} />
              {plan && (
                <Card size="small" className="shop-checkout">
                  <CheckoutForm plan={plan} methods={catalog.methods} onOrder={show} />
                </Card>
              )}
            </>
          )}
        </Layout.Content>
      </Layout>
    </ConfigProvider>
  );
}
