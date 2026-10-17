import { describe, expect, it } from 'vitest';

import type { HostedPanelView } from '@/generated/zod';
import { hostedPanelState, hostedPanelUrl, suggestHostedPorts } from '@/pages/panels/panelState';

function panel(over: Partial<HostedPanelView> = {}): HostedPanelView {
  return {
    id: 1,
    name: 'A',
    enable: true,
    username: 'a',
    webPort: 20000,
    subPort: 20001,
    webBasePath: '/a/',
    portMin: 30000,
    portMax: 30999,
    maxInbounds: 0,
    maxClients: 0,
    trafficBytes: 0,
    expiresAt: 0,
    usageEpoch: 0,
    createdAt: 1,
    process: { running: true, pid: 9, restarts: 0, lastError: '', startedAt: 1000 },
    status: {
      inbounds: 0,
      clients: 0,
      usedBytes: 0,
      blocked: '',
      xrayRunning: true,
      webTls: false,
      updatedAt: 2000,
    },
    ...over,
  };
}

describe('hostedPanelState', () => {
  it('tells a crash apart from a panel that is still coming up', () => {
    const down = { running: false, pid: 0, restarts: 2, startedAt: 1000 };
    expect(hostedPanelState(panel({ process: { ...down, lastError: 'exit status 1' } }))).toBe(
      'crashed',
    );
    expect(hostedPanelState(panel({ process: { ...down, lastError: '' } }))).toBe('starting');
  });

  it('ignores a suspension reported before the current process started', () => {
    const stale = panel({
      process: { running: true, pid: 9, restarts: 1, lastError: '', startedAt: 5000 },
      status: { ...panel().status, blocked: 'traffic', updatedAt: 4000 },
    });
    expect(hostedPanelState(stale)).toBe('starting');
    expect(hostedPanelState({ ...stale, status: { ...stale.status, updatedAt: 6000 } })).toBe(
      'suspended',
    );
  });

  it('reports a disabled panel as off whatever its last process did', () => {
    const off = panel({ enable: false });
    off.process = { ...off.process, running: false, lastError: 'signal: interrupt' };
    expect(hostedPanelState(off)).toBe('disabled');
  });
});

describe('hostedPanelUrl', () => {
  it('links over https only once the hosted panel reports a certificate', () => {
    const p = panel();
    expect(hostedPanelUrl(p, 'vpn.example.com')).toBe('http://vpn.example.com:20000/a/');
    p.status = { ...p.status, webTls: true };
    expect(hostedPanelUrl(p, 'vpn.example.com')).toBe('https://vpn.example.com:20000/a/');
  });
});

describe('suggestHostedPorts', () => {
  it('skips ports and ranges already taken by the master or other panels', () => {
    const taken = [panel({ webPort: 20000, subPort: 20001, portMin: 30000, portMax: 30999 })];
    expect(suggestHostedPorts(taken, [20010])).toEqual({
      webPort: 20020,
      subPort: 20021,
      portMin: 31000,
      portMax: 31999,
    });
  });
});
