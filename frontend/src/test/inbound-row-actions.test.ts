import { describe, expect, it } from 'vitest';

import { buildRowActionsMenu } from '@/pages/inbounds/list/RowActions';
import type { DBInboundRecord } from '@/pages/inbounds/list/types';

function inbound(protocol: string): DBInboundRecord {
  return {
    id: 7,
    enable: true,
    remark: '',
    subSortIndex: 1,
    port: 1194,
    protocol,
    up: 0,
    down: 0,
    total: 0,
    expiryTime: 0,
    _expiryTime: null,
    nodeId: null,
    settings: { clients: [] },
    streamSettings: {},
  };
}

function menuKeys(protocol: string): string[] {
  const items = buildRowActionsMenu({
    record: inbound(protocol),
    subEnable: true,
    t: (k) => k,
    hasClients: true,
  });
  return (items ?? []).flatMap((it) => (it && 'key' in it && it.key ? [String(it.key)] : []));
}

describe('inbound row actions', () => {
  // OpenVPN has no share links: an "export links" entry would hand out an empty list.
  it('offers the openvpn profile download in place of link export', () => {
    const keys = menuKeys('openvpn');
    expect(keys).toContain('openvpnProfile');
    expect(keys).not.toContain('export');
    expect(keys).toContain('subs');
  });

  // IKEv2 and L2TP have no share links either; their logins live in the client's info.
  it.each(['ikev2', 'l2tp'])('offers no link export for %s', (protocol) => {
    const keys = menuKeys(protocol);
    expect(keys).not.toContain('export');
    expect(keys).toContain('subs');
  });

  it('keeps link export for link-based protocols', () => {
    const keys = menuKeys('vless');
    expect(keys).toContain('export');
    expect(keys).not.toContain('openvpnProfile');
  });
});
