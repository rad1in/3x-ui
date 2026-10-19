import { useCallback, useEffect, useRef, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';

import { keys } from '@/api/queryKeys';
import { DashboardLayoutSchema, type DashboardLayout } from '@/generated/zod';
import { HttpUtil } from '@/utils';
import { parseMsg } from '@/utils/zodValidate';
import { DEFAULT_WIDGETS, resolveLayout, toSaved, type WidgetState } from './dashboardLayout';

const ENDPOINT = '/panel/api/admins/me/dashboardLayout';

async function fetchLayout(): Promise<DashboardLayout | null> {
  const msg = await HttpUtil.get(ENDPOINT, undefined, { silent: true });
  if (!msg?.success) return null;
  return parseMsg(msg, DashboardLayoutSchema, 'admins/me/dashboardLayout').obj ?? null;
}

// The admin's overview arrangement, kept server side per account. Edits show at once
// and are saved in the background; the server copy seeds the state once on load.
export function useDashboardLayout() {
  const qc = useQueryClient();
  const { data, isFetched } = useQuery({
    queryKey: keys.admins.dashboardLayout(),
    queryFn: fetchLayout,
    staleTime: Infinity,
  });
  const [widgets, setWidgets] = useState<WidgetState[]>(() =>
    DEFAULT_WIDGETS.map((w) => ({ ...w })),
  );
  const seeded = useRef(false);

  useEffect(() => {
    if (!isFetched || seeded.current) return;
    seeded.current = true;
    setWidgets(resolveLayout(data));
  }, [isFetched, data]);

  const commit = useCallback(
    (next: WidgetState[]) => {
      setWidgets(next);
      const saved = toSaved(next);
      qc.setQueryData(keys.admins.dashboardLayout(), saved);
      void HttpUtil.post(ENDPOINT, saved, {
        silentSuccess: true,
        headers: { 'Content-Type': 'application/json' },
      });
    },
    [qc],
  );

  const update = useCallback(
    (edit: (current: WidgetState[]) => WidgetState[]) => commit(edit(widgets)),
    [commit, widgets],
  );
  const reset = useCallback(() => commit(DEFAULT_WIDGETS.map((w) => ({ ...w }))), [commit]);

  return { widgets, update, reset };
}
