import { describe, expect, it } from 'vitest';

import {
  applyIranRouting,
  hasIranRouting,
  isIranRule,
  missingIranOutbounds,
  removeIranRouting,
} from '@/lib/xray/iranRouting';
import type { RuleObject } from '@/schemas/routing';

const api: RuleObject = { type: 'field', inboundTag: ['api'], outboundTag: 'api' };
const privateBlock: RuleObject = { type: 'field', ip: ['geoip:private'], outboundTag: 'blocked' };
const mine: RuleObject = { type: 'field', domain: ['domain:example.com'], outboundTag: 'proxy' };

const template = () => ({
  routing: { domainStrategy: 'AsIs', rules: [api, privateBlock, mine] },
  outbounds: [{ tag: 'direct' }, { tag: 'blocked' }],
});

describe('Iran routing template', () => {
  it('goes after the api and default block rules and before the admin rules', () => {
    const next = applyIranRouting(template());
    const rules = next.routing.rules;
    expect(rules[0]).toBe(api);
    expect(rules[1]).toBe(privateBlock);
    expect(rules.slice(2, 5).every(isIranRule)).toBe(true);
    expect(rules[5]).toBe(mine);
    expect(next.routing.domainStrategy).toBe('AsIs');
  });

  it('sends Iranian domains and addresses direct and ads to blocked', () => {
    const rules = applyIranRouting(template()).routing.rules.filter(isIranRule);
    const direct = rules.filter((r) => r.outboundTag === 'direct');
    expect(direct.flatMap((r) => r.domain ?? [])).toContain('geosite:category-ir');
    expect(direct.flatMap((r) => r.ip ?? [])).toEqual(['geoip:ir']);
    expect(rules.find((r) => r.outboundTag === 'blocked')?.domain).toEqual([
      'geosite:category-ads-all',
    ]);
  });

  it('applying twice leaves one copy, and removing restores the admin rules', () => {
    const twice = applyIranRouting(applyIranRouting(template()));
    expect(twice.routing.rules.filter(isIranRule)).toHaveLength(3);
    expect(hasIranRouting(twice)).toBe(true);
    const removed = removeIranRouting(twice);
    expect(removed.routing.rules).toEqual([api, privateBlock, mine]);
    expect(hasIranRouting(removed)).toBe(false);
  });

  it('works on a template without routing', () => {
    const next = applyIranRouting<{ routing?: { rules?: RuleObject[] } }>({});
    expect(next.routing?.rules?.filter(isIranRule)).toHaveLength(3);
  });

  it('reports the outbounds the rules need but the template lacks', () => {
    expect(missingIranOutbounds(template())).toEqual([]);
    expect(missingIranOutbounds({ outbounds: [{ tag: 'direct' }] })).toEqual(['blocked']);
    expect(missingIranOutbounds({})).toEqual(['direct', 'blocked']);
  });
});
