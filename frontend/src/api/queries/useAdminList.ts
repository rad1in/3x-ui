import { useQuery } from '@tanstack/react-query';
import { z } from 'zod';

import { keys } from '@/api/queryKeys';
import { AdminViewSchema, type AdminView } from '@/generated/zod';
import { HttpUtil } from '@/utils';
import { parseMsg } from '@/utils/zodValidate';

async function fetchAdmins(): Promise<AdminView[]> {
  const msg = await HttpUtil.get('/panel/api/admins/list', undefined, { silent: true });
  if (!msg?.success) throw new Error(msg?.msg || 'Failed to load admins');
  return parseMsg(msg, z.array(AdminViewSchema), 'admins/list').obj ?? [];
}

// Owner-only on the server; pass enabled=false for any other role.
export function useAdminList(enabled = true) {
  return useQuery({ queryKey: keys.admins.list(), queryFn: fetchAdmins, enabled });
}
