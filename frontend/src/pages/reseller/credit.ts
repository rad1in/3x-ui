// Credit is the panel's own integer unit for resellers, not a currency.
export function formatCredit(amount: number): string {
  return new Intl.NumberFormat().format(amount);
}

export const REPORT_RANGES = ['all', '7', '30', '90'] as const;
export type ReportRange = (typeof REPORT_RANGES)[number];

// The report window as unix ms; 0 leaves the start open.
export function rangeStart(range: ReportRange, now: number = Date.now()): number {
  return range === 'all' ? 0 : now - Number(range) * 24 * 60 * 60 * 1000;
}
