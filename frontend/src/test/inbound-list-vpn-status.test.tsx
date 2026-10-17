import { screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { describe, expect, it, vi } from 'vitest';

import InboundList from '@/pages/inbounds/list/InboundList';
import type { DBInboundRecord, InboundListProps } from '@/pages/inbounds/list/types';
import { renderWithProviders } from '@/test/test-utils';

function l2tpInbound(id: number, remark: string): DBInboundRecord {
  return {
    id,
    enable: true,
    remark,
    subSortIndex: 1,
    port: 1701,
    protocol: 'l2tp',
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

function renderList(props: Partial<InboundListProps>) {
  const base: InboundListProps = {
    dbInbounds: [],
    clientCount: {},
    onlineClients: [],
    lastOnlineMap: {},
    inboundSpeed: {},
    expireDiff: 0,
    trafficDiff: 0,
    pageSize: 25,
    isMobile: false,
    subEnable: false,
    nodesById: new Map(),
    hasActiveNode: false,
    hosts: [],
    vpnStatuses: [],
    onAddInbound: vi.fn(),
    onGeneralAction: vi.fn(),
    onRowAction: vi.fn(),
    onBulkDelete: vi.fn(async () => true),
  };
  renderWithProviders(
    <MemoryRouter initialEntries={['/inbounds']}>
      <InboundList {...base} {...props} />
    </MemoryRouter>,
  );
}

describe('InboundList VPN status', () => {
  // A VPN server the host cannot run would otherwise look healthy in the list.
  it('flags a VPN inbound that is not running, with the reason', async () => {
    const reason = 'this kernel has no PPP support (/dev/ppp is missing), which L2TP needs';
    renderList({
      dbInbounds: [l2tpInbound(4, 'phones'), l2tpInbound(5, 'tablets')],
      vpnStatuses: [
        { id: 4, running: false, reason },
        { id: 5, running: true },
      ],
    });
    const flags = screen.getAllByTestId('vpn-down');
    expect(flags).toHaveLength(1);
    expect(flags[0].closest('tr')?.textContent).toContain('phones');
    expect(flags[0].getAttribute('aria-label')).toBe(reason);
  });
});
