import { useTranslation } from 'react-i18next';
import { Progress, Tooltip, Typography } from 'antd';

import { SizeFormatter } from '@/utils';
import type { NodeRecord } from '@/api/queries/useNodesQuery';

// Below this much traffic the cost-per-TB figure swings too wildly to mean anything.
const MIN_BYTES_FOR_RATE = 10 * SizeFormatter.ONE_GB;

export function nodeCostPerTb(node: NodeRecord): number | null {
  const used = node.periodBytes ?? 0;
  const cost = node.monthlyCost ?? 0;
  if (cost <= 0 || used < MIN_BYTES_FOR_RATE) return null;
  return cost / (used / SizeFormatter.ONE_TB);
}

// Sums the monthly cost per currency; amounts in different currencies are never added together.
export function monthlyCostTotals(nodes: NodeRecord[]): { currency: string; amount: number }[] {
  const byCurrency = new Map<string, number>();
  for (const n of nodes) {
    if (n.transitive || !n.monthlyCost) continue;
    const currency = n.costCurrency ?? '';
    byCurrency.set(currency, (byCurrency.get(currency) ?? 0) + n.monthlyCost);
  }
  return [...byCurrency].map(([currency, amount]) => ({ currency, amount }));
}

export default function NodeUsageCell({ node }: { node: NodeRecord }) {
  const { t } = useTranslation();
  const used = node.periodBytes ?? 0;
  const capGb = node.monthlyCapGB ?? 0;
  const capBytes = capGb * SizeFormatter.ONE_GB;
  const perTb = nodeCostPerTb(node);
  const currency = node.costCurrency || '';

  return (
    <div style={{ minWidth: 130 }}>
      <Tooltip title={t('pages.nodes.metered.usageHint')}>
        <span>
          {SizeFormatter.sizeFormat(used)}
          {capGb > 0 && ` / ${SizeFormatter.sizeFormat(capBytes)}`}
        </span>
      </Tooltip>
      {capGb > 0 && (
        <Progress
          percent={Math.min(100, Math.round((used / capBytes) * 100))}
          size="small"
          status={node.capState === 'reached' ? 'exception' : 'normal'}
          showInfo={false}
        />
      )}
      {(node.monthlyCost ?? 0) > 0 && (
        <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block' }}>
          {`${node.monthlyCost} ${currency}`.trim()} {t('pages.nodes.metered.perMonth')}
          {perTb !== null && ` · ${perTb.toFixed(2)} ${currency} ${t('pages.nodes.metered.perTb')}`}
        </Typography.Text>
      )}
    </div>
  );
}
