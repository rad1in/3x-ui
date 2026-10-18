import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { z } from 'zod';

import { keys } from '@/api/queryKeys';
import {
  OrderPageSchema,
  PlanSchema,
  SalesConfigSchema,
  type OrderPage,
  type Plan,
  type SalesConfig,
} from '@/generated/zod';
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

async function fetchSalesConfig(): Promise<SalesConfig> {
  const msg = await HttpUtil.get('/panel/api/sales/config', undefined, { silent: true });
  if (!msg?.success) throw new Error(msg?.msg || 'Failed to load sales settings');
  return parseMsg(msg, SalesConfigSchema, 'sales/config', { strict: true }).obj as SalesConfig;
}

export function useSalesConfig() {
  const queryClient = useQueryClient();
  const query = useQuery({ queryKey: keys.sales.config(), queryFn: fetchSalesConfig });
  const save = useMutation({
    mutationFn: (cfg: SalesConfig) =>
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
