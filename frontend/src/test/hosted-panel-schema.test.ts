import { describe, expect, it } from 'vitest';

import { hostedPanelFormSchema, type HostedPanelFormValues } from '@/schemas/hostedPanel';

const valid: HostedPanelFormValues = {
  name: 'A',
  enable: true,
  username: 'reseller',
  password: '',
  webPort: 20000,
  subPort: 20001,
  webBasePath: '/a/',
  portMin: 30000,
  portMax: 30999,
  maxInbounds: 0,
  maxClients: 0,
  trafficGB: 0,
  expiresAt: 0,
};

function issues(original: { username: string } | null, v: Partial<HostedPanelFormValues>) {
  const res = hostedPanelFormSchema(original).safeParse({ ...valid, ...v });
  return res.success ? [] : res.error.issues.map((i) => `${i.path.join('.')}:${i.message}`);
}

describe('hostedPanelFormSchema', () => {
  it('needs a password on create but keeps the old one on a plain edit', () => {
    expect(issues(null, {})).toEqual(['password:pages.hostedPanels.errPassword']);
    expect(issues({ username: 'reseller' }, {})).toEqual([]);
  });

  it('needs a new password when the login is renamed, as the server does', () => {
    expect(issues({ username: 'reseller' }, { username: 'renamed' })).toEqual([
      'password:pages.hostedPanels.errPassword',
    ]);
    expect(
      issues({ username: 'reseller' }, { username: 'renamed', password: 'long-enough' }),
    ).toEqual([]);
  });

  it('keeps the panel address out of its own inbound range', () => {
    expect(issues({ username: 'reseller' }, { webPort: 30500 })).toEqual([
      'webPort:pages.hostedPanels.errAddrInRange',
    ]);
  });
});
