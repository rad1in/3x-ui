import { useQuery } from '@tanstack/react-query';

import { keys } from '@/api/queryKeys';
import { HostedViewSchema, type HostedView } from '@/generated/zod';
import { HttpUtil } from '@/utils';
import { parseMsg } from '@/utils/zodValidate';

async function fetchHosted(): Promise<HostedView | null> {
  const msg = await HttpUtil.get('/panel/api/hosted/status', undefined, { silent: true });
  if (!msg?.success) return null;
  return parseMsg(msg, HostedViewSchema, 'hosted/status').obj ?? null;
}

// enabled is true only inside a panel that another panel hosts.
export function useHostedStatus(options: { poll?: boolean } = {}) {
  return useQuery({
    queryKey: keys.hosted.status(),
    queryFn: fetchHosted,
    refetchInterval: options.poll ? 30_000 : false,
    staleTime: options.poll ? 0 : 60_000,
  });
}
