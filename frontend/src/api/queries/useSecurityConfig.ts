import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { keys } from '@/api/queryKeys';
import { SecurityConfigViewSchema, type SecurityConfigView } from '@/generated/zod';
import { HttpUtil } from '@/utils';
import { parseMsg } from '@/utils/zodValidate';

const JSON_HEADERS = { headers: { 'Content-Type': 'application/json' } } as const;

export function useSecurityConfig() {
  const queryClient = useQueryClient();
  const query = useQuery({
    queryKey: keys.security.config(),
    queryFn: async (): Promise<SecurityConfigView> => {
      const msg = await HttpUtil.get('/panel/api/security/config', undefined, { silent: true });
      if (!msg?.success) throw new Error(msg?.msg || 'Failed to load the security settings');
      return parseMsg(msg, SecurityConfigViewSchema, 'security/config').obj as SecurityConfigView;
    },
  });
  const save = useMutation({
    mutationFn: (cfg: Record<string, unknown>) =>
      HttpUtil.post('/panel/api/security/config/save', cfg, JSON_HEADERS),
    onSuccess: (msg) => {
      if (msg?.success) void queryClient.invalidateQueries({ queryKey: keys.security.config() });
    },
  });
  return { config: query.data, save: save.mutateAsync, saving: save.isPending };
}
