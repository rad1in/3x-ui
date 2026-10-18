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
  clearSecrets: z.array(z.string()),
});
export type SalesConfigValues = z.infer<typeof salesConfigSchema>;
