import type { RuleObject } from '@/schemas/routing';

export const IRAN_RULE_TAG_PREFIX = 'iran-template:';

const DIRECT = 'direct';
const BLOCKED = 'blocked';

interface TemplateLike {
  routing?: { rules?: RuleObject[] } & Record<string, unknown>;
  outbounds?: Array<{ tag?: string }>;
}

export function iranRoutingRules(): RuleObject[] {
  return [
    {
      type: 'field',
      ruleTag: `${IRAN_RULE_TAG_PREFIX}ads`,
      domain: ['geosite:category-ads-all'],
      outboundTag: BLOCKED,
    },
    {
      type: 'field',
      ruleTag: `${IRAN_RULE_TAG_PREFIX}domains`,
      domain: ['regexp:.*\\.ir$', 'geosite:category-ir'],
      outboundTag: DIRECT,
    },
    {
      type: 'field',
      ruleTag: `${IRAN_RULE_TAG_PREFIX}ips`,
      ip: ['geoip:ir'],
      outboundTag: DIRECT,
    },
  ];
}

export function isIranRule(rule: RuleObject): boolean {
  return rule.ruleTag?.startsWith(IRAN_RULE_TAG_PREFIX) === true;
}

// Rules that must keep matching first: the panel's own API and the default blocks.
function isLeadingGuard(rule: RuleObject): boolean {
  return rule.outboundTag === 'api' || (rule.outboundTag === BLOCKED && !rule.domain);
}

export function missingIranOutbounds(template: TemplateLike): string[] {
  const tags = new Set((template.outbounds ?? []).map((o) => o.tag));
  return [DIRECT, BLOCKED].filter((tag) => !tags.has(tag));
}

export function hasIranRouting(template: TemplateLike): boolean {
  return (template.routing?.rules ?? []).some(isIranRule);
}

// Replaces any earlier copy of the template, so applying twice never duplicates it.
export function applyIranRouting<T extends TemplateLike>(template: T): T {
  const kept = (template.routing?.rules ?? []).filter((r) => !isIranRule(r));
  let at = 0;
  while (at < kept.length && isLeadingGuard(kept[at])) at++;
  const rules = [...kept.slice(0, at), ...iranRoutingRules(), ...kept.slice(at)];
  return { ...template, routing: { ...template.routing, rules } };
}

export function removeIranRouting<T extends TemplateLike>(template: T): T {
  const rules = (template.routing?.rules ?? []).filter((r) => !isIranRule(r));
  return { ...template, routing: { ...template.routing, rules } };
}
