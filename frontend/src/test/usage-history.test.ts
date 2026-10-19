import { describe, expect, it } from 'vitest';

import { fillDailyUsage } from '@/lib/traffic/usage-history';

describe('fillDailyUsage', () => {
  it('returns one point per day ending today, with zero for days without a row', () => {
    const rows = [
      { day: '2026-10-16', up: 10, down: 20 },
      { day: '2026-10-18', up: 1, down: 2 },
    ];
    expect(fillDailyUsage(rows, 4, new Date(2026, 9, 18))).toEqual([
      { day: '2026-10-15', up: 0, down: 0 },
      { day: '2026-10-16', up: 10, down: 20 },
      { day: '2026-10-17', up: 0, down: 0 },
      { day: '2026-10-18', up: 1, down: 2 },
    ]);
  });

  it('crosses a month boundary using calendar days', () => {
    const days = fillDailyUsage([], 3, new Date(2026, 10, 1)).map((p) => p.day);
    expect(days).toEqual(['2026-10-30', '2026-10-31', '2026-11-01']);
  });
});
