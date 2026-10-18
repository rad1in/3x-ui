import type { DiscountCode } from '@/generated/zod';
import { fromMinor, toMinor, type Currency } from '@/lib/money';
import type { DiscountFormValues } from '@/schemas/sales';

export function blankDiscount(): DiscountFormValues {
  return {
    code: '',
    kind: 'percent',
    value: 10,
    currency: 'USD',
    planIds: [],
    maxUses: 0,
    oncePerCustomer: false,
    expiresAt: 0,
    enabled: true,
  };
}

export function discountToForm(code: DiscountCode): DiscountFormValues {
  const currency = (code.currency || 'USD') as Currency;
  return {
    id: code.id,
    code: code.code,
    kind: code.kind === 'fixed' ? 'fixed' : 'percent',
    value: code.kind === 'fixed' ? fromMinor(code.value, currency) : code.value,
    currency,
    planIds: code.planIds ?? [],
    maxUses: code.maxUses,
    oncePerCustomer: code.oncePerCustomer,
    expiresAt: code.expiresAt,
    enabled: code.enabled,
  };
}

export function formToDiscount(values: DiscountFormValues): Partial<DiscountCode> {
  const fixed = values.kind === 'fixed';
  return {
    id: values.id ?? 0,
    code: values.code,
    kind: values.kind,
    value: fixed ? toMinor(values.value, values.currency) : values.value,
    currency: fixed ? values.currency : '',
    planIds: values.planIds,
    maxUses: values.maxUses,
    oncePerCustomer: values.oncePerCustomer,
    expiresAt: values.expiresAt,
    enabled: values.enabled,
  };
}
