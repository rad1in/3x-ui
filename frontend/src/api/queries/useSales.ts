import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { z } from 'zod';

import { keys } from '@/api/queryKeys';
import {
  DiscountCodeSchema,
  OrderPageSchema,
  PlanSchema,
  WalletViewSchema,
  SalesConfigViewSchema,
  type DiscountCode,
  type OrderPage,
  type Plan,
  type WalletView,
  type SalesConfigView,
} from '@/generated/zod';
import type { SalesConfigValues } from '@/schemas/sales';
import { HttpUtil } from '@/utils';
import { parseMsg } from '@/utils/zodValidate';

const JSON_HEADERS = { headers: { 'Content-Type': 'application/json' } } as const;
export const ORDER_PAGE_SIZE = 20;

async function fetchPlans(): Promise<Plan[]> {
  const msg = await HttpUtil.get('/panel/api/sales/plans', undefined, { silent: true });
  if (!msg?.success) throw new Error(msg?.msg || 'Failed to load plans');
  return parseMsg(msg, z.array(PlanSchema), 'sales/plans').obj ?? [];
}

async function fetchOrders(status: string, page: number): Promise<OrderPage> {
  const msg = await HttpUtil.get(
    '/panel/api/sales/orders',
    { status, limit: ORDER_PAGE_SIZE, offset: (page - 1) * ORDER_PAGE_SIZE },
    { silent: true },
  );
  if (!msg?.success) throw new Error(msg?.msg || 'Failed to load orders');
  return parseMsg(msg, OrderPageSchema, 'sales/orders').obj ?? { items: [], total: 0 };
}

export function usePlans() {
  const queryClient = useQueryClient();
  const query = useQuery({ queryKey: keys.sales.plans(), queryFn: fetchPlans });
  const refresh = () => queryClient.invalidateQueries({ queryKey: keys.sales.plans() });

  const save = useMutation({
    mutationFn: (plan: Partial<Plan>) =>
      HttpUtil.post('/panel/api/sales/plans/save', plan, JSON_HEADERS),
    onSuccess: (msg) => {
      if (msg?.success) void refresh();
    },
  });
  const remove = useMutation({
    mutationFn: (id: number) => HttpUtil.post(`/panel/api/sales/plans/del/${id}`),
    onSuccess: (msg) => {
      if (msg?.success) void refresh();
    },
  });

  return {
    plans: query.data ?? [],
    loading: query.isLoading,
    save: save.mutateAsync,
    saving: save.isPending,
    remove: remove.mutateAsync,
  };
}

async function fetchDiscounts(): Promise<DiscountCode[]> {
  const msg = await HttpUtil.get('/panel/api/sales/discounts', undefined, { silent: true });
  if (!msg?.success) throw new Error(msg?.msg || 'Failed to load discount codes');
  return parseMsg(msg, z.array(DiscountCodeSchema), 'sales/discounts').obj ?? [];
}

export function useDiscounts() {
  const queryClient = useQueryClient();
  const query = useQuery({ queryKey: keys.sales.discounts(), queryFn: fetchDiscounts });
  const refresh = (msg: { success?: boolean } | undefined) => {
    if (msg?.success) void queryClient.invalidateQueries({ queryKey: keys.sales.discounts() });
  };
  const save = useMutation({
    mutationFn: (code: Partial<DiscountCode>) =>
      HttpUtil.post('/panel/api/sales/discounts/save', code, JSON_HEADERS),
    onSuccess: refresh,
  });
  const remove = useMutation({
    mutationFn: (id: number) => HttpUtil.post(`/panel/api/sales/discounts/del/${id}`),
    onSuccess: refresh,
  });
  return {
    discounts: query.data ?? [],
    loading: query.isLoading,
    save: save.mutateAsync,
    saving: save.isPending,
    remove: remove.mutateAsync,
  };
}

export function useWallet(email: string) {
  const queryClient = useQueryClient();
  const query = useQuery({
    queryKey: keys.sales.wallet(email),
    enabled: email !== '',
    queryFn: async (): Promise<WalletView> => {
      const msg = await HttpUtil.get('/panel/api/sales/wallet', { email }, { silent: true });
      if (!msg?.success) throw new Error(msg?.msg || 'Failed to load the wallet');
      return parseMsg(msg, WalletViewSchema, 'sales/wallet', { strict: true }).obj as WalletView;
    },
  });
  const adjust = useMutation({
    mutationFn: (body: { email: string; currency: string; amount: number; note: string }) =>
      HttpUtil.post('/panel/api/sales/wallet/adjust', body, JSON_HEADERS),
    onSuccess: (msg) => {
      if (msg?.success) void queryClient.invalidateQueries({ queryKey: keys.sales.root() });
    },
  });
  return {
    wallet: query.data,
    loading: query.isFetching,
    error: query.error,
    adjust: adjust.mutateAsync,
    adjusting: adjust.isPending,
  };
}

export function useOrderActions() {
  const queryClient = useQueryClient();
  const refresh = (msg: { success?: boolean } | undefined) => {
    if (!msg?.success) return;
    void queryClient.invalidateQueries({ queryKey: keys.sales.root() });
    void queryClient.invalidateQueries({ queryKey: keys.clients.root() });
  };
  const approve = useMutation({
    mutationFn: (id: number) => HttpUtil.post(`/panel/api/sales/orders/approve/${id}`),
    onSettled: (msg) => refresh(msg),
  });
  const reject = useMutation({
    mutationFn: (args: { id: number; reason: string }) =>
      HttpUtil.post(
        `/panel/api/sales/orders/reject/${args.id}`,
        { reason: args.reason },
        JSON_HEADERS,
      ),
    onSettled: (msg) => refresh(msg),
  });
  return { approve: approve.mutateAsync, reject: reject.mutateAsync };
}

async function fetchSalesConfig(): Promise<SalesConfigView> {
  const msg = await HttpUtil.get('/panel/api/sales/config', undefined, { silent: true });
  if (!msg?.success) throw new Error(msg?.msg || 'Failed to load sales settings');
  return parseMsg(msg, SalesConfigViewSchema, 'sales/config', { strict: true })
    .obj as SalesConfigView;
}

export function useSalesConfig() {
  const queryClient = useQueryClient();
  const query = useQuery({ queryKey: keys.sales.config(), queryFn: fetchSalesConfig });
  const save = useMutation({
    mutationFn: (cfg: SalesConfigValues) =>
      HttpUtil.post('/panel/api/sales/config/save', cfg, JSON_HEADERS),
    onSuccess: (msg) => {
      if (msg?.success) void queryClient.invalidateQueries({ queryKey: keys.sales.config() });
    },
  });
  return {
    config: query.data,
    loading: query.isLoading,
    save: save.mutateAsync,
    saving: save.isPending,
  };
}

export function useOrders(status: string, page: number) {
  return useQuery({
    queryKey: keys.sales.orders(status, page),
    queryFn: () => fetchOrders(status, page),
  });
}
