import { useQuery } from '@tanstack/react-query';

import { keys } from '@/api/queryKeys';
import { VPNStatusSchema, type VPNStatus } from '@/generated/zod';
import { HttpUtil } from '@/utils';
import { parseMsg } from '@/utils/zodValidate';
import { z } from 'zod';

async function fetchVPNStatus(): Promise<VPNStatus[]> {
  const msg = await HttpUtil.get('/panel/api/inbounds/vpnStatus', undefined, { silent: true });
  if (!msg?.success) return [];
  return parseMsg(msg, z.array(VPNStatusSchema), 'inbounds/vpnStatus').obj ?? [];
}

// Polled only while the list holds an OpenVPN, IKEv2 or L2TP inbound.
export function useVPNStatus(enabled: boolean) {
  return useQuery({
    queryKey: keys.inbounds.vpnStatus(),
    queryFn: fetchVPNStatus,
    enabled,
    refetchInterval: 15_000,
  });
}
