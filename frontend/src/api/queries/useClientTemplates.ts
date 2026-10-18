import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { HttpUtil } from '@/utils';
import { parseMsg } from '@/utils/zodValidate';
import {
  ClientTemplateApplyResultSchema,
  ClientTemplateListSchema,
  ClientTemplateSchema,
  type ClientTemplate,
} from '@/schemas/clientTemplate';
import { keys } from '@/api/queryKeys';

const JSON_HEADERS = { headers: { 'Content-Type': 'application/json' } } as const;

async function fetchTemplates(): Promise<ClientTemplate[]> {
  const msg = await HttpUtil.get('/panel/api/clients/templates', undefined, { silent: true });
  if (!msg?.success) throw new Error(msg?.msg || 'Failed to fetch client templates');
  const validated = parseMsg(msg, ClientTemplateListSchema, 'clients/templates');
  return Array.isArray(validated.obj) ? validated.obj : [];
}

export function useClientTemplates(enabled = true) {
  const queryClient = useQueryClient();
  const query = useQuery({
    queryKey: keys.clients.templates(),
    queryFn: fetchTemplates,
    enabled,
  });

  const refresh = () => queryClient.invalidateQueries({ queryKey: keys.clients.templates() });

  const saveMut = useMutation({
    mutationFn: async (template: ClientTemplate) => {
      const raw = await HttpUtil.post('/panel/api/clients/templates/save', template, JSON_HEADERS);
      return parseMsg(raw, ClientTemplateSchema, 'clients/templates/save');
    },
    onSuccess: (msg) => {
      if (msg?.success) void refresh();
    },
  });

  const deleteMut = useMutation({
    mutationFn: (id: number) => HttpUtil.post(`/panel/api/clients/templates/del/${id}`),
    onSuccess: (msg) => {
      if (msg?.success) void refresh();
    },
  });

  const applyMut = useMutation({
    mutationFn: async (body: { templateId: number; emails: string[] }) => {
      const raw = await HttpUtil.post('/panel/api/clients/templates/apply', body, JSON_HEADERS);
      return parseMsg(raw, ClientTemplateApplyResultSchema, 'clients/templates/apply');
    },
    onSuccess: (msg) => {
      if (!msg?.success) return;
      void queryClient.invalidateQueries({ queryKey: keys.clients.root() });
      void queryClient.invalidateQueries({ queryKey: keys.inbounds.root() });
    },
  });

  return {
    templates: query.data ?? [],
    loading: query.isFetching,
    save: saveMut.mutateAsync,
    remove: deleteMut.mutateAsync,
    apply: applyMut.mutateAsync,
    applying: applyMut.isPending,
  };
}
