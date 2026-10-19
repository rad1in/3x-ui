import { describe, expect, it } from 'vitest';

import { reportUrl } from '@/lib/report-download';

describe('reportUrl', () => {
  it('keeps the panel base path and drops empty filters', () => {
    expect(
      reportUrl('panel/api/sales/orders/export', { format: 'xlsx', status: '' }, '/secret/'),
    ).toBe('/secret/panel/api/sales/orders/export?format=xlsx');
  });

  it('encodes values that need it', () => {
    expect(reportUrl('panel/api/sales/orders/export', { status: 'a b&c' }, '/')).toBe(
      '/panel/api/sales/orders/export?status=a+b%26c',
    );
  });
});
