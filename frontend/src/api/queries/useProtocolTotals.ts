import { useQuery } from '@tanstack/react-query';
import { z } from 'zod';

import { keys } from '@/api/queryKeys';
import { totalsByProtocol, type ProtocolTotal } from '@/lib/traffic/protocol-totals';
import { HttpUtil } from '@/utils';
import { parseMsg } from '@/utils/zodValidate';

const InboundTrafficListSchema = z.array(
  z
    .object({ protocol: z.string(), up: z.number().optional(), down: z.number().optional() })
    .loose(),
);

async function fetchProtocolTotals(): Promise<ProtocolTotal[]> {
  const msg = await HttpUtil.get('/panel/api/inbounds/list/slim', undefined, { silent: true });
  if (!msg?.success) return [];
  const parsed = parseMsg(msg, InboundTrafficListSchema, 'inbounds/list/slim');
  return totalsByProtocol(parsed.obj ?? []);
}

export function useProtocolTotals() {
  return useQuery({
    queryKey: keys.inbounds.protocolTotals(),
    queryFn: fetchProtocolTotals,
    refetchInterval: 30_000,
  });
}
