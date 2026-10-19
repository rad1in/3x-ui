import { useQuery } from '@tanstack/react-query';

import { keys } from '@/api/queryKeys';
import { HealthReportSchema, type HealthReport } from '@/generated/zod';
import { HttpUtil } from '@/utils';
import { parseMsg } from '@/utils/zodValidate';

async function fetchHealth(): Promise<HealthReport | null> {
  const msg = await HttpUtil.get('/panel/api/server/health', undefined, { silent: true });
  if (!msg?.success) return null;
  return parseMsg(msg, HealthReportSchema, 'server/health').obj ?? null;
}

export function useHealth() {
  return useQuery({
    queryKey: keys.server.health(),
    queryFn: fetchHealth,
    refetchInterval: 30_000,
  });
}
