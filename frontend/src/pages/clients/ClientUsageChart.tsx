import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { Divider } from 'antd';
import { z } from 'zod';

import { HttpUtil, SizeFormatter } from '@/utils';
import { parseMsg } from '@/utils/zodValidate';
import { Sparkline } from '@/components/viz';
import { fillDailyUsage } from '@/lib/traffic/usage-history';

const USAGE_DAYS = 30;

const UsageRowsSchema = z.array(z.object({ day: z.string(), up: z.number(), down: z.number() }));

export default function ClientUsageChart({ email }: { email: string }) {
  const { t } = useTranslation();
  const query = useQuery({
    queryKey: ['clients', 'usage', email, USAGE_DAYS],
    queryFn: async () => {
      const msg = await HttpUtil.get(
        `/panel/api/clients/usage/${encodeURIComponent(email)}`,
        { days: USAGE_DAYS },
        { silent: true },
      );
      if (!msg?.success) throw new Error(msg?.msg || 'Failed to fetch usage history');
      return parseMsg(msg, UsageRowsSchema, 'clients/usage').obj ?? [];
    },
    enabled: email !== '',
    select: (rows) => fillDailyUsage(rows, USAGE_DAYS, new Date()),
  });

  const points = query.data ?? [];
  const total = points.reduce((sum, p) => sum + p.up + p.down, 0);
  if (total === 0) return null;

  return (
    <>
      <Divider>{t('pages.clients.usageHistory.title')}</Divider>
      <Sparkline
        data={points.map((p) => p.up)}
        data2={points.map((p) => p.down)}
        name1={t('pages.clients.usageHistory.upload')}
        name2={t('pages.clients.usageHistory.download')}
        labels={points.map((p) => p.day)}
        height={120}
        showAxes
        showTooltip
        showLegend
        yFormatter={(v) => SizeFormatter.sizeFormat(v)}
        tooltipFormatter={(v) => SizeFormatter.sizeFormat(v)}
      />
    </>
  );
}
