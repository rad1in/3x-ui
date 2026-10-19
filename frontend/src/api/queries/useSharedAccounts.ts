import { useQuery } from '@tanstack/react-query';
import { z } from 'zod';

import { keys } from '@/api/queryKeys';
import { SharedAccountFlagSchema, type SharedAccountFlag } from '@/generated/zod';
import { HttpUtil } from '@/utils';
import { parseMsg } from '@/utils/zodValidate';

async function fetchSharedAccounts(): Promise<SharedAccountFlag[]> {
  const msg = await HttpUtil.get('/panel/api/clients/sharedAccounts', undefined, { silent: true });
  if (!msg?.success) return [];
  return parseMsg(msg, z.array(SharedAccountFlagSchema), 'clients/sharedAccounts').obj ?? [];
}

export function useSharedAccounts() {
  return useQuery({
    queryKey: keys.sharedAccounts(),
    queryFn: fetchSharedAccounts,
    refetchInterval: 60_000,
  });
}
