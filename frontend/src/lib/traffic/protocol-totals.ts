export interface InboundTraffic {
  protocol: string;
  up?: number;
  down?: number;
}

export interface ProtocolTotal {
  protocol: string;
  up: number;
  down: number;
  total: number;
  share: number;
}

// Largest first; protocols that moved no bytes are left out.
export function totalsByProtocol(inbounds: InboundTraffic[]): ProtocolTotal[] {
  const sums = new Map<string, { up: number; down: number }>();
  for (const ib of inbounds) {
    const cur = sums.get(ib.protocol) ?? { up: 0, down: 0 };
    cur.up += ib.up ?? 0;
    cur.down += ib.down ?? 0;
    sums.set(ib.protocol, cur);
  }
  const rows = [...sums].map(([protocol, s]) => ({ protocol, ...s, total: s.up + s.down }));
  const grand = rows.reduce((n, r) => n + r.total, 0);
  return rows
    .filter((r) => r.total > 0)
    .map((r) => ({ ...r, share: r.total / grand }))
    .sort((a, b) => b.total - a.total || a.protocol.localeCompare(b.protocol));
}
