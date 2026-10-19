export const CURRENCIES = ['IRT', 'IRR', 'USD', 'EUR', 'USDT'] as const;
export type Currency = (typeof CURRENCIES)[number];

const DECIMALS: Record<Currency, number> = { IRT: 0, IRR: 0, USD: 2, EUR: 2, USDT: 2 };

export function currencyDecimals(currency: string): number {
  return DECIMALS[currency as Currency] ?? 2;
}

// Prices travel as integers in the minor unit; rounding absorbs 19.99 * 100 drift.
export function toMinor(major: number, currency: string): number {
  return Math.round(major * 10 ** currencyDecimals(currency));
}

export function fromMinor(minor: number, currency: string): number {
  return minor / 10 ** currencyDecimals(currency);
}

export function formatMoney(minor: number, currency: string, locale?: string): string {
  const digits = currencyDecimals(currency);
  const number = new Intl.NumberFormat(locale, {
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  }).format(fromMinor(minor, currency));
  return `${number} ${currency}`;
}
