import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { z } from 'zod';

import { keys } from '@/api/queryKeys';
import { OrderPageSchema, PlanSchema, type OrderPage, type Plan } from '@/generated/zod';
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

export function useOrders(status: string, page: number) {
  return useQuery({
    queryKey: keys.sales.orders(status, page),
    queryFn: () => fetchOrders(status, page),
  });
}
