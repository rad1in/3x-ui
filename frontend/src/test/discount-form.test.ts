import { describe, expect, it } from 'vitest';

import { blankDiscount, discountToForm, formToDiscount } from '@/lib/discount';
import { discountFormSchema } from '@/schemas/sales';

describe('discount form conversion', () => {
  it('sends a fixed amount in minor units and a percent as is, clearing the currency of a percent code', () => {
    const fixed = {
      ...blankDiscount(),
      code: 'FIVE',
      kind: 'fixed' as const,
      value: 4.99,
      currency: 'USD' as const,
    };
    expect(formToDiscount(fixed)).toMatchObject({ kind: 'fixed', value: 499, currency: 'USD' });

    const percent = {
      ...blankDiscount(),
      code: 'TEN',
      kind: 'percent' as const,
      value: 10,
      currency: 'USD' as const,
    };
    expect(formToDiscount(percent)).toMatchObject({ kind: 'percent', value: 10, currency: '' });
  });

  it('reads a stored fixed discount back into major units', () => {
    const form = discountToForm({
      id: 3,
      code: 'FIVE',
      kind: 'fixed',
      value: 499,
      currency: 'USD',
      planIds: [1],
      maxUses: 0,
      oncePerCustomer: true,
      expiresAt: 0,
      enabled: true,
      uses: 0,
      createdAt: 0,
      updatedAt: 0,
    });
    expect(form).toMatchObject({ id: 3, value: 4.99, currency: 'USD', oncePerCustomer: true });
  });

  it('refuses a percent that is not a whole number from 1 to 99', () => {
    const base = { ...blankDiscount(), code: 'TEN' };
    expect(discountFormSchema.safeParse({ ...base, value: 10 }).success).toBe(true);
    expect(discountFormSchema.safeParse({ ...base, value: 100 }).success).toBe(false);
    expect(discountFormSchema.safeParse({ ...base, value: 12.5 }).success).toBe(false);
    expect(discountFormSchema.safeParse({ ...base, kind: 'fixed', value: 12.5 }).success).toBe(
      true,
    );
  });
});
