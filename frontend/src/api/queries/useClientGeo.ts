import { useQuery } from '@tanstack/react-query';

import { keys } from '@/api/queryKeys';
import { ClientGeoSchema, type ClientGeo } from '@/generated/zod';
import { HttpUtil } from '@/utils';
import { parseMsg } from '@/utils/zodValidate';

async function fetchClientGeo(): Promise<ClientGeo | null> {
  const msg = await HttpUtil.get('/panel/api/clients/countries', undefined, { silent: true });
  if (!msg?.success) return null;
  return parseMsg(msg, ClientGeoSchema, 'clients/countries').obj ?? null;
}

export function useClientGeo() {
  return useQuery({
    queryKey: keys.clients.geo(),
    queryFn: fetchClientGeo,
    refetchInterval: 30_000,
  });
}
