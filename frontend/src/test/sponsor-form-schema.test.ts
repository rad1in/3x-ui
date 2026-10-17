import { describe, expect, it } from 'vitest';

import { SponsorFormSchema, type SponsorFormValues } from '@/schemas/sponsor';

const valid: SponsorFormValues = {
  name: 'Acme',
  enable: true,
  slots: ['dashboard'],
  from: 0,
  until: 0,
  title: '',
  text: '',
  link: 'https://acme.example/',
  logo: '',
};

function firstIssue(values: SponsorFormValues): string | undefined {
  const r = SponsorFormSchema.safeParse(values);
  return r.success ? undefined : r.error.issues[0]?.message;
}

describe('SponsorFormSchema', () => {
  it('accepts a minimal sponsor', () => {
    expect(firstIssue(valid)).toBeUndefined();
  });

  it.each([
    ['blank name', { name: '  ' }, 'pages.sponsors.admin.errName'],
    ['http link', { link: 'http://acme.example/' }, 'pages.sponsors.admin.errLink'],
    ['javascript link', { link: 'javascript:alert(1)' }, 'pages.sponsors.admin.errLink'],
    ['no slot', { slots: [] }, 'pages.sponsors.admin.errSlots'],
    ['end before start', { from: 2000, until: 1000 }, 'pages.sponsors.admin.errWindow'],
  ])('rejects %s with a translated message', (_n, patch, key) => {
    expect(firstIssue({ ...valid, ...patch } as SponsorFormValues)).toBe(key);
  });
});
