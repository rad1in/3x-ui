import { describe, expect, it } from 'vitest';

import { countryFlag, countryName } from '@/lib/country';

describe('country helpers', () => {
  it('turns a two-letter code into its flag, whatever the case', () => {
    expect(countryFlag('DE')).toBe('🇩🇪');
    expect(countryFlag('br')).toBe('🇧🇷');
  });

  it('gives no flag for geoip categories that are not countries', () => {
    expect(countryFlag('CLOUDFLARE')).toBe('');
    expect(countryFlag('')).toBe('');
  });

  it('names a country in the requested language and falls back to the code', () => {
    expect(countryName('DE', 'en')).toBe('Germany');
    expect(countryName('DE', 'de')).toBe('Deutschland');
    expect(countryName('CLOUDFLARE', 'en')).toBe('CLOUDFLARE');
  });
});
