import { describe, expect, it } from 'vitest';

import type { NodeRecord } from '@/api/queries/useNodesQuery';
import { monthlyCostTotals, nodeCostPerTb } from '@/pages/nodes/NodeUsageCell';

const GB = 1024 ** 3;
const node = (extra: Partial<NodeRecord>): NodeRecord => ({ id: 1, ...extra }) as NodeRecord;

describe('monthlyCostTotals', () => {
  it('sums per currency without mixing them and skips read-only sub-nodes', () => {
    const totals = monthlyCostTotals([
      node({ monthlyCost: 5, costCurrency: 'USD' }),
      node({ monthlyCost: 7.5, costCurrency: 'USD' }),
      node({ monthlyCost: 4, costCurrency: 'EUR' }),
      node({ monthlyCost: 99, costCurrency: 'USD', transitive: true }),
      node({ monthlyCost: 0, costCurrency: 'USD' }),
    ]);
    expect(totals).toEqual([
      { currency: 'USD', amount: 12.5 },
      { currency: 'EUR', amount: 4 },
    ]);
  });
});

describe('nodeCostPerTb', () => {
  it('divides the monthly cost by the terabytes used so far', () => {
    expect(nodeCostPerTb(node({ monthlyCost: 10, periodBytes: 512 * GB }))).toBe(20);
  });

  it('withholds the figure until enough traffic has flowed to mean something', () => {
    expect(nodeCostPerTb(node({ monthlyCost: 10, periodBytes: 5 * GB }))).toBeNull();
    expect(nodeCostPerTb(node({ monthlyCost: 0, periodBytes: 512 * GB }))).toBeNull();
  });
});
