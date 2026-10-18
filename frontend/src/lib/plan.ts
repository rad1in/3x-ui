import type { Plan } from '@/generated/zod';
import { fromMinor, toMinor, type Currency } from '@/lib/money';
import type { PlanFormValues } from '@/schemas/sales';

const BYTES_PER_GB = 1024 ** 3;

export function blankPlan(templateId: number): PlanFormValues {
  return {
    name: '',
    description: '',
    price: 0,
    currency: 'IRT',
    totalGb: 0,
    durationDays: 30,
    limitIp: 0,
    templateId,
    enabled: true,
    sortOrder: 0,
  };
}

export function planToForm(plan: Plan): PlanFormValues {
  return {
    id: plan.id,
    name: plan.name,
    description: plan.description,
    price: fromMinor(plan.price, plan.currency),
    currency: plan.currency as Currency,
    totalGb: plan.totalGB / BYTES_PER_GB,
    durationDays: plan.durationDays,
    limitIp: plan.limitIp,
    templateId: plan.templateId,
    enabled: plan.enabled,
    sortOrder: plan.sortOrder,
  };
}

export function formToPlan(values: PlanFormValues): Partial<Plan> {
  return {
    id: values.id ?? 0,
    name: values.name,
    description: values.description,
    price: toMinor(values.price, values.currency),
    currency: values.currency,
    totalGB: Math.round(values.totalGb * BYTES_PER_GB),
    durationDays: values.durationDays,
    limitIp: values.limitIp,
    templateId: values.templateId,
    enabled: values.enabled,
    sortOrder: values.sortOrder,
  };
}
