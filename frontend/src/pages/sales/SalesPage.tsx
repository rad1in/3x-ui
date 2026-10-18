import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Card, ConfigProvider, Layout, Tabs, message } from 'antd';
import { ShoppingCartOutlined } from '@ant-design/icons';

import { useCurrentAdmin } from '@/api/queries/useCurrentAdmin';
import { usePageTitle } from '@/hooks/usePageTitle';
import { useTheme } from '@/hooks/useTheme';
import AppSidebar from '@/layouts/AppSidebar';
import { setMessageInstance } from '@/utils/messageBus';
import DiscountsTab from './DiscountsTab';
import OrdersTab from './OrdersTab';
import PlansTab from './PlansTab';
import SalesSettingsTab from './SalesSettingsTab';
import WalletsTab from './WalletsTab';
import './SalesPage.css';

export default function SalesPage() {
  const { t } = useTranslation();
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  const [messageApi, messageContextHolder] = message.useMessage();
  usePageTitle();
  const isOwner = useCurrentAdmin().data?.role === 'owner';
  useEffect(() => {
    setMessageInstance(messageApi);
  }, [messageApi]);

  const pageClass = `sales-page ${isDark ? 'is-dark' : ''} ${isUltra ? 'is-ultra' : ''}`.trim();

  return (
    <ConfigProvider theme={antdThemeConfig}>
      {messageContextHolder}
      <Layout className={pageClass}>
        <AppSidebar />
        <Layout className="content-shell">
          <Layout.Content className="content-area">
            <Card
              size="small"
              title={
                <span className="sales-title">
                  <ShoppingCartOutlined /> {t('menu.sales')}
                </span>
              }
            >
              <p className="sales-hint" dir="auto">
                {t('pages.sales.hint')}
              </p>
              <Tabs
                items={[
                  { key: 'plans', label: t('pages.sales.plansTab'), children: <PlansTab /> },
                  { key: 'orders', label: t('pages.sales.ordersTab'), children: <OrdersTab /> },
                  {
                    key: 'discounts',
                    label: t('pages.sales.discountsTab'),
                    children: <DiscountsTab />,
                  },
                  { key: 'wallets', label: t('pages.sales.walletsTab'), children: <WalletsTab /> },
                  ...(isOwner
                    ? [
                        {
                          key: 'settings',
                          label: t('pages.sales.settingsTab'),
                          children: <SalesSettingsTab />,
                        },
                      ]
                    : []),
                ]}
              />
            </Card>
          </Layout.Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  );
}
