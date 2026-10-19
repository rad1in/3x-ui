import { describe, expect, it } from 'vitest';

import { formatMoney, toMinor } from '@/lib/money';
import { blankPlan, formToPlan, planToForm } from '@/lib/plan';

describe('plan money', () => {
  it('rounds 19.99 USD to exactly 1999 cents', () => {
    expect(19.99 * 100).not.toBe(1999);
    expect(toMinor(19.99, 'USD')).toBe(1999);
  });

  it('keeps whole-unit currencies without decimals', () => {
    expect(toMinor(250000, 'IRT')).toBe(250000);
    expect(formatMoney(250000, 'IRT', 'en-US')).toBe('250,000 IRT');
    expect(formatMoney(1999, 'USD', 'en-US')).toBe('19.99 USD');
  });

  it('round-trips a plan through the form without drifting the price or quota', () => {
    const wire = {
      id: 3,
      name: 'Pro',
      description: '',
      price: 1999,
      currency: 'USD',
      totalGB: 50 * 1024 ** 3,
      durationDays: 30,
      limitIp: 2,
      templateId: 4,
      enabled: false,
      sortOrder: 1,
      createdAt: 0,
      updatedAt: 0,
    };
    const form = planToForm(wire);
    expect(form.price).toBe(19.99);
    expect(form.totalGb).toBe(50);
    expect(formToPlan(form)).toMatchObject({
      id: 3,
      price: 1999,
      totalGB: 50 * 1024 ** 3,
      enabled: false,
    });
  });

  it('starts a new plan with id unset so the server creates it', () => {
    expect(formToPlan(blankPlan(7)).id).toBe(0);
  });
});
