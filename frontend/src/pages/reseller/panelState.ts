import type { HostedPanelView } from '@/generated/zod';

export type HostedPanelState = 'disabled' | 'starting' | 'crashed' | 'suspended' | 'running';

export function hostedPanelState(p: HostedPanelView): HostedPanelState {
  if (!p.enable) return 'disabled';
  if (!p.process.running) return p.process.lastError ? 'crashed' : 'starting';
  // status.json outlives restarts; a report older than this process is not about it.
  if (p.status.updatedAt < p.process.startedAt) return 'starting';
  return p.status.blocked ? 'suspended' : 'running';
}

export function hostedPanelUrl(p: HostedPanelView, host: string): string {
  const scheme = p.status.webTls ? 'https' : 'http';
  return `${scheme}://${host}:${p.webPort}${p.webBasePath}`;
}

const WEB_PORT_START = 20000;
const WEB_PORT_STEP = 10;
const RANGE_START = 30000;
const RANGE_SIZE = 1000;

function claimed(panels: HostedPanelView[], port: number): boolean {
  return panels.some(
    (p) => port === p.webPort || port === p.subPort || (port >= p.portMin && port <= p.portMax),
  );
}

export interface HostedPortSuggestion {
  webPort: number;
  subPort: number;
  portMin: number;
  portMax: number;
}

// A starting point for the create form; the server still checks every collision.
export function suggestHostedPorts(
  panels: HostedPanelView[],
  reserved: number[],
): HostedPortSuggestion {
  const free = (port: number) => !reserved.includes(port) && !claimed(panels, port);
  let webPort = WEB_PORT_START;
  while (webPort < RANGE_START && !(free(webPort) && free(webPort + 1))) webPort += WEB_PORT_STEP;

  let portMin = RANGE_START;
  const overlaps = (lo: number, hi: number) =>
    panels.some((p) => lo <= p.portMax && p.portMin <= hi) ||
    reserved.some((port) => port >= lo && port <= hi);
  while (portMin + RANGE_SIZE <= 65536 && overlaps(portMin, portMin + RANGE_SIZE - 1)) {
    portMin += RANGE_SIZE;
  }
  return { webPort, subPort: webPort + 1, portMin, portMax: portMin + RANGE_SIZE - 1 };
}
