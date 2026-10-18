import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { keys } from '@/api/queryKeys';
import {
  ResellerAccountSchema,
  ResellerReportSchema,
  type ResellerAccount,
  type ResellerReport,
} from '@/generated/zod';
import { HttpUtil } from '@/utils';
import { parseMsg } from '@/utils/zodValidate';

const JSON_HEADERS = { headers: { 'Content-Type': 'application/json' } } as const;

async function fetchAccount(url: string, label: string): Promise<ResellerAccount> {
  const msg = await HttpUtil.get(url, undefined, { silent: true });
  if (!msg?.success) throw new Error(msg?.msg || 'Failed to load the reseller account');
  return parseMsg(msg, ResellerAccountSchema, label).obj as ResellerAccount;
}

export function useResellerMe(enabled: boolean) {
  return useQuery({
    queryKey: keys.reseller.me(),
    enabled,
    queryFn: () => fetchAccount('/panel/api/reseller/me', 'reseller/me'),
  });
}

export function useResellerAccount(id: number | null) {
  return useQuery({
    queryKey: keys.reseller.account(id ?? 0),
    enabled: id !== null,
    queryFn: () => fetchAccount(`/panel/api/reseller/account/${id}`, 'reseller/account'),
  });
}

// from and to are unix ms; 0 leaves that end of the window open.
export function useResellerReport(from: number, to: number) {
  return useQuery({
    queryKey: keys.reseller.report(from, to),
    queryFn: async (): Promise<ResellerReport> => {
      const msg = await HttpUtil.get(
        '/panel/api/reseller/report',
        { from: from || undefined, to: to || undefined },
        { silent: true },
      );
      if (!msg?.success) throw new Error(msg?.msg || 'Failed to load the sales report');
      return parseMsg(msg, ResellerReportSchema, 'reseller/report').obj as ResellerReport;
    },
  });
}

export function useResellerActions() {
  const queryClient = useQueryClient();
  const refresh = (msg: { success?: boolean } | undefined) => {
    if (!msg?.success) return;
    void queryClient.invalidateQueries({ queryKey: keys.reseller.root() });
    void queryClient.invalidateQueries({ queryKey: keys.admins.list() });
  };
  const topup = useMutation({
    mutationFn: (v: { id: number; amount: number; note: string }) =>
      HttpUtil.post(
        `/panel/api/reseller/topup/${v.id}`,
        { amount: v.amount, note: v.note },
        JSON_HEADERS,
      ),
    onSuccess: refresh,
  });
  const setPrices = useMutation({
    mutationFn: (v: { id: number; pricePerGb: number | null; pricePerDay: number | null }) =>
      HttpUtil.post(
        `/panel/api/reseller/prices/${v.id}`,
        { pricePerGb: v.pricePerGb, pricePerDay: v.pricePerDay },
        JSON_HEADERS,
      ),
    onSuccess: refresh,
  });
  return {
    topup: topup.mutateAsync,
    toppingUp: topup.isPending,
    setPrices: setPrices.mutateAsync,
    savingPrices: setPrices.isPending,
  };
}
