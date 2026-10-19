import { z } from 'zod';

import { CURRENCIES } from '@/lib/money';

// Amounts are entered in major units (19.99); lib/money converts on save.
export const planFormSchema = z.object({
  id: z.number().int().optional(),
  name: z.string().trim().min(1, 'pages.sales.plans.errName'),
  description: z.string(),
  price: z.number().min(0, 'pages.sales.plans.errPrice'),
  currency: z.enum(CURRENCIES),
  totalGb: z.number().min(0),
  durationDays: z.number().int().min(0),
  limitIp: z.number().int().min(0),
  templateId: z.number().int().positive('pages.sales.plans.errTemplate'),
  enabled: z.boolean(),
  sortOrder: z.number().int(),
});
export type PlanFormValues = z.infer<typeof planFormSchema>;

export const ORDER_STATUSES = [
  'pending',
  'review',
  'paid',
  'rejected',
  'failed',
  'canceled',
] as const;
export type OrderStatus = (typeof ORDER_STATUSES)[number];

export const TICKET_STATUSES = ['open', 'answered', 'closed'] as const;

export const salesConfigSchema = z.object({
  publicEnabled: z.boolean(),
  publicBaseUrl: z.string().trim(),
  card: z.object({
    enabled: z.boolean(),
    cardNumber: z.string(),
    holderName: z.string(),
    bankName: z.string(),
    instructions: z.string(),
  }),
  zarinpal: z.object({ enabled: z.boolean(), merchantId: z.string(), sandbox: z.boolean() }),
  idpay: z.object({ enabled: z.boolean(), apiKey: z.string(), sandbox: z.boolean() }),
  crypto: z.object({
    enabled: z.boolean(),
    apiKey: z.string(),
    ipnSecret: z.string(),
    sandbox: z.boolean(),
  }),
  stripe: z.object({ enabled: z.boolean(), secretKey: z.string(), webhookSecret: z.string() }),
  paypal: z.object({
    enabled: z.boolean(),
    clientId: z.string(),
    clientSecret: z.string(),
    sandbox: z.boolean(),
  }),
  walletEnabled: z.boolean(),
  telegramShop: z.boolean(),
  referral: z.object({
    enabled: z.boolean(),
    bonusPercent: z
      .number()
      .int()
      .min(0, 'pages.sales.config.errBonus')
      .max(100, 'pages.sales.config.errBonus'),
  }),
  clearSecrets: z.array(z.string()),
});
export type SalesConfigValues = z.infer<typeof salesConfigSchema>;

export const DISCOUNT_KINDS = ['percent', 'fixed'] as const;

// A fixed value is entered in major units of its currency; lib/discount converts.
export const discountFormSchema = z
  .object({
    id: z.number().int().optional(),
    code: z.string().trim().min(3, 'pages.sales.discounts.errCode'),
    kind: z.enum(DISCOUNT_KINDS),
    value: z.number().positive('pages.sales.discounts.errValue'),
    currency: z.enum(CURRENCIES),
    planIds: z.array(z.number().int()),
    maxUses: z.number().int().min(0),
    oncePerCustomer: z.boolean(),
    expiresAt: z.number().int().min(0),
    enabled: z.boolean(),
  })
  .refine((v) => v.kind !== 'percent' || (Number.isInteger(v.value) && v.value <= 99), {
    path: ['value'],
    message: 'pages.sales.discounts.errPercent',
  });
export type DiscountFormValues = z.infer<typeof discountFormSchema>;
