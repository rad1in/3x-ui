import { z } from 'zod';

import {
  QuoteSchema,
  ShopCatalogSchema,
  ShopOrderViewSchema,
  ShopWalletSchema,
  type Quote,
  type ShopCatalog,
  type ShopOrderView,
  type ShopWallet,
} from '@/generated/zod';

export type ShopResult<T> = { ok: true; data: T } | { ok: false; code: string };

const envelope = z.object({
  success: z.boolean(),
  msg: z.string().optional(),
  obj: z.unknown().optional(),
});

// The shop answers refusals as stable codes in msg; anything unreadable is
// reported as a network code so the page can show one generic message.
async function call<T>(
  method: 'GET' | 'POST',
  path: string,
  schema: z.ZodType<T>,
  body?: unknown,
): Promise<ShopResult<T>> {
  try {
    const res = await fetch(`/shop/api${path}`, {
      method,
      headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
      body: body === undefined ? undefined : JSON.stringify(body),
      cache: 'no-store',
    });
    const parsed = envelope.safeParse(await res.json());
    if (!parsed.success) return { ok: false, code: 'network' };
    if (!parsed.data.success) return { ok: false, code: parsed.data.msg || 'network' };
    const data = schema.safeParse(parsed.data.obj);
    return data.success ? { ok: true, data: data.data } : { ok: false, code: 'network' };
  } catch {
    return { ok: false, code: 'network' };
  }
}

export interface OrderInput {
  planId: number;
  kind: 'new' | 'renew';
  email: string;
  gateway: string;
  contact?: string;
  discountCode?: string;
  referralCode?: string;
  proof?: string;
}

export const shopApi = {
  catalog: (): Promise<ShopResult<ShopCatalog>> => call('GET', '/plans', ShopCatalogSchema),
  quote: (body: {
    planId: number;
    discountCode: string;
    email: string;
  }): Promise<ShopResult<Quote>> => call('POST', '/quote', QuoteSchema, body),
  createOrder: (body: OrderInput): Promise<ShopResult<ShopOrderView>> =>
    call('POST', '/orders', ShopOrderViewSchema, body),
  order: (token: string): Promise<ShopResult<ShopOrderView>> =>
    call('GET', `/orders/${encodeURIComponent(token)}`, ShopOrderViewSchema),
  receipt: (token: string, ref: string, note: string): Promise<ShopResult<ShopOrderView>> =>
    call('POST', `/orders/${encodeURIComponent(token)}/receipt`, ShopOrderViewSchema, {
      ref,
      note,
    }),
  wallet: (email: string, proof: string): Promise<ShopResult<ShopWallet>> =>
    call(
      'GET',
      `/wallet?email=${encodeURIComponent(email)}&proof=${encodeURIComponent(proof)}`,
      ShopWalletSchema,
    ),
};

export const SHOP_ERROR_CODES = [
  'plan_unavailable',
  'invalid_email',
  'invalid_kind',
  'email_taken',
  'email_unknown',
  'gateway_unavailable',
  'order_not_found',
  'order_state',
  'receipt_invalid',
  'currency_unsupported',
  'gateway_failed',
  'discount_invalid',
  'discount_used',
  'referral_invalid',
  'wallet_insufficient',
  'proof_invalid',
  'rate_limited',
  'network',
] as const;

// An unknown code (a newer server) falls back to the generic message.
export function errorKey(code: string): string {
  return (SHOP_ERROR_CODES as readonly string[]).includes(code)
    ? `shop.errors.${code}`
    : 'shop.errors.network';
}
