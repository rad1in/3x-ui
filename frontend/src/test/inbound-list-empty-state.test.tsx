import { fireEvent, screen, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { describe, expect, it, vi } from 'vitest';

import InboundList from '@/pages/inbounds/list/InboundList';
import type { DBInboundRecord, InboundListProps } from '@/pages/inbounds/list/types';
import { renderWithProviders } from '@/test/test-utils';

function inbound(id: number, remark: string): DBInboundRecord {
  return {
    id,
    enable: true,
    remark,
    subSortIndex: 1,
    port: 40000 + id,
    protocol: 'vless',
    up: 0,
    down: 0,
    total: 0,
    expiryTime: 0,
    _expiryTime: null,
    nodeId: null,
    settings: { clients: [] },
    streamSettings: {},
    isVLess: true,
  };
}

function renderList(props: Partial<InboundListProps>, url = '/inbounds') {
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
    onAddInbound: vi.fn(),
    onGeneralAction: vi.fn(),
    onRowAction: vi.fn(),
    onBulkDelete: vi.fn(async () => true),
  };
  const merged = { ...base, ...props };
  renderWithProviders(
    <MemoryRouter initialEntries={[url]}>
      <InboundList {...merged} />
    </MemoryRouter>,
  );
  return merged;
}

describe('InboundList empty states', () => {
  it('offers to create the first inbound when the panel has none', () => {
    const props = renderList({ dbInbounds: [] });

    const empty = screen.getByTestId('empty-state');
    expect(within(empty).getByText('No inbounds yet')).toBeTruthy();
    expect(within(empty).getByText('Create an inbound')).toBeTruthy();
    fireEvent.click(within(empty).getByRole('button', { name: /Add Inbound/ }));
    expect(props.onAddInbound).toHaveBeenCalledTimes(1);
  });

  it('tells a search miss apart from an empty panel and clears it back', () => {
    renderList({ dbInbounds: [inbound(1, 'edge-de')] }, '/inbounds?search=nomatch');

    const empty = screen.getByTestId('empty-state');
    expect(within(empty).queryByText('No inbounds yet')).toBeNull();
    expect(within(empty).getByText('No results match your search or filters')).toBeTruthy();
    fireEvent.click(within(empty).getByRole('button', { name: /Clear/ }));
    expect(screen.getByText('edge-de')).toBeTruthy();
  });
});
