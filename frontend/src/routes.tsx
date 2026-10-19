import { lazy, Suspense } from 'react';
import { createBrowserRouter, Navigate, type RouteObject } from 'react-router';
import { Spin } from 'antd';

import PanelLayout from '@/layouts/PanelLayout';

const HomeRoute = lazy(() => import('@/pages/index/HomeRoute'));
const InboundsPage = lazy(() => import('@/pages/inbounds/InboundsPage'));
const ClientsPage = lazy(() => import('@/pages/clients/ClientsPage'));
const GroupsPage = lazy(() => import('@/pages/groups/GroupsPage'));
const NodesPage = lazy(() => import('@/pages/nodes/NodesPage'));
const HostsPage = lazy(() => import('@/pages/hosts/HostsPage'));
const SettingsPage = lazy(() => import('@/pages/settings/SettingsPage'));
const XrayPage = lazy(() => import('@/pages/xray/XrayPage'));
const ApiDocsPage = lazy(() => import('@/pages/api-docs/ApiDocsPage'));
const SponsorsPage = lazy(() => import('@/pages/sponsors/SponsorsPage'));
const SalesPage = lazy(() => import('@/pages/sales/SalesPage'));
const AuditPage = lazy(() => import('@/pages/audit/AuditPage'));
const AdminsPage = lazy(() => import('@/pages/admins/AdminsPage'));
const SessionsPage = lazy(() => import('@/pages/sessions/SessionsPage'));
const ResellerPage = lazy(() => import('@/pages/reseller/ResellerPage'));

function withSuspense(node: React.ReactNode) {
  return (
    <Suspense
      fallback={
        <div
          style={{
            display: 'flex',
            justifyContent: 'center',
            alignItems: 'center',
            minHeight: '60vh',
          }}
        >
          <Spin size="large" />
        </div>
      }
    >
      {node}
    </Suspense>
  );
}

const routes: RouteObject[] = [
  {
    path: '/',
    element: <PanelLayout />,
    children: [
      { index: true, element: withSuspense(<HomeRoute />) },
      { path: 'inbounds', element: withSuspense(<InboundsPage />) },
      { path: 'clients', element: withSuspense(<ClientsPage />) },
      { path: 'groups', element: withSuspense(<GroupsPage />) },
      { path: 'nodes', element: withSuspense(<NodesPage />) },
      { path: 'hosts', element: withSuspense(<HostsPage />) },
      { path: 'settings', element: withSuspense(<SettingsPage />) },
      { path: 'xray', element: withSuspense(<XrayPage />) },
      { path: 'outbound', element: withSuspense(<XrayPage />) },
      { path: 'routing', element: withSuspense(<XrayPage />) },
      { path: 'api-docs', element: withSuspense(<ApiDocsPage />) },
      { path: 'sponsors', element: withSuspense(<SponsorsPage />) },
      { path: 'sales', element: withSuspense(<SalesPage />) },
      { path: 'audit', element: withSuspense(<AuditPage />) },
      { path: 'admins', element: withSuspense(<AdminsPage />) },
      { path: 'sessions', element: withSuspense(<SessionsPage />) },
      { path: 'reseller', element: withSuspense(<ResellerPage />) },
      { path: 'panels', element: <Navigate to="/reseller" replace /> },
    ],
  },
];

function computeBasename() {
  const raw = (typeof window !== 'undefined' && window.X_UI_BASE_PATH) || '/';
  const trimmed = raw.replace(/\/+$/, '');
  return `${trimmed}/panel`;
}

export const router = createBrowserRouter(routes, {
  basename: computeBasename(),
});
