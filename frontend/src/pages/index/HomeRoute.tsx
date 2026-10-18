import { lazy } from 'react';
import { Navigate } from 'react-router';

import { useCurrentAdmin } from '@/api/queries/useCurrentAdmin';
import { homePage } from '@/lib/adminRole';

const IndexPage = lazy(() => import('@/pages/index/IndexPage'));

// The dashboard reads server-wide data an operator may not see.
export default function HomeRoute() {
  const { data: me, isPending } = useCurrentAdmin();
  if (isPending) return null;
  const home = homePage(me?.role);
  return home === '/' ? <IndexPage /> : <Navigate to={home} replace />;
}
