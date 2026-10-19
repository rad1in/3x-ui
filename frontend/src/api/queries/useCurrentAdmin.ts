import { useQuery } from '@tanstack/react-query';

import { keys } from '@/api/queryKeys';
import { AdminViewSchema, type AdminView } from '@/generated/zod';
import { HttpUtil } from '@/utils';
import { parseMsg } from '@/utils/zodValidate';

async function fetchCurrentAdmin(): Promise<AdminView | null> {
  const msg = await HttpUtil.get('/panel/api/admins/me', undefined, { silent: true });
  if (!msg?.success) return null;
  return parseMsg(msg, AdminViewSchema, 'admins/me').obj ?? null;
}

// The signed-in admin; the server enforces the role, this only shapes the UI.
export function useCurrentAdmin() {
  return useQuery({
    queryKey: keys.admins.me(),
    queryFn: fetchCurrentAdmin,
    staleTime: 60_000,
  });
}
