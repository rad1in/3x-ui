import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, List, Space, Tag, Typography } from 'antd';

import { HttpUtil, IntlUtil } from '@/utils';
import { useCurrentAdmin } from '@/api/queries/useCurrentAdmin';

interface ApprovalRow {
  id: number;
  action: string;
  target: string;
  requestedBy: number;
  requestedByName: string;
  requestedAt: number;
  expiresAt: number;
  status: string;
  decidedByName: string;
  result: string;
}

const STATUS_COLOR: Record<string, string> = {
  pending: 'gold',
  approved: 'green',
  rejected: 'red',
  cancelled: 'default',
  expired: 'default',
  failed: 'red',
};

// Pending two-person approvals; the server refuses decisions by the requester.
export default function ApprovalsPanel() {
  const { t } = useTranslation();
  const { data: admin } = useCurrentAdmin();
  const [rows, setRows] = useState<ApprovalRow[]>([]);
  const [loading, setLoading] = useState(true);
  const canDecide = admin?.role === 'owner' || admin?.role === 'admin';

  const load = useCallback(async () => {
    try {
      const msg = await HttpUtil.get('/panel/api/approvals/list', undefined, { silent: true });
      if (msg?.success) setRows(Array.isArray(msg.obj) ? (msg.obj as ApprovalRow[]) : []);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  async function decide(row: ApprovalRow, verb: 'approve' | 'reject' | 'cancel') {
    await HttpUtil.post(`/panel/api/approvals/${verb}/${row.id}`);
    await load();
  }

  const actionLabel: Record<string, string> = {
    'inbound.delete': t('pages.approvals.inboundDelete'),
    'clients.bulkDelete': t('pages.approvals.clientsBulkDelete'),
    'node.delete': t('pages.approvals.nodeDelete'),
  };
  const statusLabel: Record<string, string> = {
    pending: t('pages.approvals.statusPending'),
    approved: t('pages.approvals.statusApproved'),
    rejected: t('pages.approvals.statusRejected'),
    cancelled: t('pages.approvals.statusCancelled'),
    expired: t('pages.approvals.statusExpired'),
    failed: t('pages.approvals.statusFailed'),
  };

  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <Typography.Paragraph type="secondary">{t('pages.approvals.intro')}</Typography.Paragraph>
      <List
        loading={loading}
        bordered
        dataSource={rows}
        locale={{ emptyText: t('pages.approvals.empty') }}
        renderItem={(row) => {
          const own = row.requestedBy === admin?.id;
          return (
            <List.Item
              actions={
                row.status === 'pending' && canDecide
                  ? own
                    ? [
                        <Button
                          key="cancel"
                          size="small"
                          onClick={() => void decide(row, 'cancel')}
                        >
                          {t('pages.approvals.cancel')}
                        </Button>,
                      ]
                    : [
                        <Button
                          key="approve"
                          size="small"
                          type="primary"
                          onClick={() => void decide(row, 'approve')}
                        >
                          {t('pages.approvals.approve')}
                        </Button>,
                        <Button
                          key="reject"
                          size="small"
                          danger
                          onClick={() => void decide(row, 'reject')}
                        >
                          {t('pages.approvals.reject')}
                        </Button>,
                      ]
                  : undefined
              }
            >
              <List.Item.Meta
                title={
                  <Space>
                    <span>{actionLabel[row.action] ?? row.action}</span>
                    <Tag color={STATUS_COLOR[row.status]}>
                      {statusLabel[row.status] ?? row.status}
                    </Tag>
                  </Space>
                }
                description={
                  <>
                    <div>
                      {row.target} · {t('pages.approvals.requestedBy')}: {row.requestedByName} ·{' '}
                      {t('pages.approvals.expires')}: {IntlUtil.formatDate(row.expiresAt)}
                    </div>
                    {row.status === 'pending' && own && (
                      <div>{t('pages.approvals.ownRequestHint')}</div>
                    )}
                    {row.result && (
                      <div>
                        {t('pages.approvals.resultLabel')}: {row.result}
                      </div>
                    )}
                  </>
                }
              />
            </List.Item>
          );
        }}
      />
    </Space>
  );
}
