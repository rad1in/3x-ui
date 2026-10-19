import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { z } from 'zod';

import { keys } from '@/api/queryKeys';
import {
  CleanIPResultSchema,
  CleanIPStatusSchema,
  type CleanIPResult,
  type CleanIPStatus,
} from '@/generated/zod';
import { HttpUtil } from '@/utils';
import { parseMsg } from '@/utils/zodValidate';

const JSON_HEADERS = { headers: { 'Content-Type': 'application/json' } } as const;

export interface CleanIPScanValues {
  targets: string;
  port: number;
  sni: string;
  samples: number;
}

export interface CleanIPApplyValues {
  ip: string;
  groupId?: string;
  inboundId?: number;
  mode: 'replace' | 'add';
}

async function fetchStatus(): Promise<CleanIPStatus> {
  const msg = await HttpUtil.get('/panel/api/hosts/cleanip/status', undefined, { silent: true });
  if (!msg?.success) throw new Error(msg?.msg || 'Failed to load scan status');
  return parseMsg(msg, CleanIPStatusSchema, 'hosts/cleanip/status').obj as CleanIPStatus;
}

async function fetchResults(): Promise<CleanIPResult[]> {
  const msg = await HttpUtil.get('/panel/api/hosts/cleanip/results', undefined, { silent: true });
  if (!msg?.success) throw new Error(msg?.msg || 'Failed to load scan results');
  return parseMsg(msg, z.array(CleanIPResultSchema), 'hosts/cleanip/results').obj ?? [];
}

export function useCleanIP(enabled: boolean) {
  const queryClient = useQueryClient();
  const status = useQuery({
    queryKey: keys.hosts.cleanIpStatus(),
    queryFn: fetchStatus,
    enabled,
    refetchInterval: (query) => (query.state.data?.running ? 1000 : false),
  });
  const finishedAt = status.data?.finishedAt ?? 0;
  const results = useQuery({
    queryKey: keys.hosts.cleanIpResults(finishedAt),
    queryFn: fetchResults,
    enabled: enabled && status.data !== undefined && !status.data.running,
  });

  const start = useMutation({
    mutationFn: (values: CleanIPScanValues) =>
      HttpUtil.post('/panel/api/hosts/cleanip/scan', values, JSON_HEADERS),
    onSuccess: (msg) => {
      if (msg?.success)
        void queryClient.invalidateQueries({ queryKey: keys.hosts.cleanIpStatus() });
    },
  });

  const apply = useMutation({
    mutationFn: (values: CleanIPApplyValues) =>
      HttpUtil.post('/panel/api/hosts/cleanip/apply', values, JSON_HEADERS),
    onSuccess: (msg) => {
      if (msg?.success) void queryClient.invalidateQueries({ queryKey: keys.hosts.list() });
    },
  });

  return { status: status.data, results: results.data ?? [], start, apply };
}
