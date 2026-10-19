import { describe, expect, it } from 'vitest';

import { totalsByProtocol } from '@/lib/traffic/protocol-totals';

describe('totalsByProtocol', () => {
  it('sums inbounds of one protocol, ranks by bytes and gives each its share of the total', () => {
    const rows = totalsByProtocol([
      { protocol: 'vless', up: 100, down: 200 },
      { protocol: 'trojan', up: 10, down: 20 },
      { protocol: 'vless', up: 300, down: 400 },
      { protocol: 'vmess' },
    ]);
    expect(rows).toEqual([
      { protocol: 'vless', up: 400, down: 600, total: 1000, share: 1000 / 1030 },
      { protocol: 'trojan', up: 10, down: 20, total: 30, share: 30 / 1030 },
    ]);
  });

  it('is empty when nothing has moved, instead of dividing by zero', () => {
    expect(totalsByProtocol([{ protocol: 'vless', up: 0, down: 0 }])).toEqual([]);
  });
});
