import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Drawer, Input, Popconfirm, Select, Space, Table, Tag, Typography } from 'antd';
import type { TableColumnsType } from 'antd';

import { TICKET_PAGE_SIZE, useTicket, useTicketActions, useTickets } from '@/api/queries/useSales';
import type { Ticket } from '@/generated/zod';
import { TICKET_STATUSES } from '@/schemas/sales';

const STATUS_COLORS: Record<string, string> = {
  open: 'gold',
  answered: 'green',
  closed: 'default',
};

export default function TicketsTab() {
  const { t } = useTranslation();
  const [status, setStatus] = useState('');
  const [page, setPage] = useState(1);
  const [openId, setOpenId] = useState<number | null>(null);
  const [draft, setDraft] = useState('');
  const { data, isLoading } = useTickets(status, page);
  const { data: detail } = useTicket(openId);
  const { reply, replying, close, reopen } = useTicketActions();

  async function onReply() {
    if (openId === null) return;
    const msg = await reply({ id: openId, body: draft });
    if (msg?.success) setDraft('');
  }

  const columns: TableColumnsType<Ticket> = [
    { title: '#', dataIndex: 'id', width: 70 },
    {
      title: t('pages.sales.tickets.subject'),
      dataIndex: 'subject',
      render: (subject: string, row) => (
        <Typography.Link onClick={() => setOpenId(row.id)}>
          <span dir="auto">{subject}</span>
        </Typography.Link>
      ),
    },
    {
      title: t('pages.sales.tickets.client'),
      dataIndex: 'clientEmail',
      render: (email: string) => <span dir="auto">{email}</span>,
    },
    {
      title: t('pages.sales.tickets.status'),
      dataIndex: 'status',
      render: (s: string) => (
        <Tag color={STATUS_COLORS[s]}>{t(`pages.sales.tickets.statuses.${s}`)}</Tag>
      ),
    },
    {
      title: t('pages.sales.tickets.updated'),
      dataIndex: 'updatedAt',
      render: (ms: number) => new Date(ms).toLocaleString(),
    },
  ];

  const ticket = detail?.ticket;
  return (
    <>
      <div className="sales-toolbar">
        <Select
          value={status}
          style={{ minWidth: 180 }}
          onChange={(v) => {
            setStatus(v);
            setPage(1);
          }}
          options={[
            { value: '', label: t('pages.sales.tickets.allStatuses') },
            ...TICKET_STATUSES.map((s) => ({
              value: s,
              label: t(`pages.sales.tickets.statuses.${s}`),
            })),
          ]}
        />
      </div>
      <Table<Ticket>
        rowKey="id"
        size="small"
        loading={isLoading}
        columns={columns}
        dataSource={data?.items ?? []}
        scroll={{ x: 640 }}
        pagination={{
          current: page,
          pageSize: TICKET_PAGE_SIZE,
          total: data?.total ?? 0,
          showSizeChanger: false,
          onChange: setPage,
        }}
      />
      <Drawer
        open={openId !== null}
        width={520}
        title={ticket ? `#${ticket.id} ${ticket.subject}` : undefined}
        onClose={() => setOpenId(null)}
        destroyOnHidden
      >
        {ticket && detail && (
          <>
            <Space style={{ marginBottom: 12 }}>
              <span dir="auto">{ticket.clientEmail}</span>
              <Tag color={STATUS_COLORS[ticket.status]}>
                {t(`pages.sales.tickets.statuses.${ticket.status}`)}
              </Tag>
              {ticket.status === 'closed' ? (
                <Button size="small" onClick={() => reopen(ticket.id)}>
                  {t('pages.sales.tickets.reopen')}
                </Button>
              ) : (
                <Popconfirm
                  title={t('pages.sales.tickets.confirmClose')}
                  onConfirm={() => close(ticket.id)}
                >
                  <Button size="small" danger>
                    {t('pages.sales.tickets.close')}
                  </Button>
                </Popconfirm>
              )}
            </Space>
            <div className="ticket-thread">
              {detail.messages.map((m) => (
                <div
                  key={m.id}
                  dir="auto"
                  className={m.author === 'admin' ? 'ticket-message is-admin' : 'ticket-message'}
                >
                  {m.body}
                </div>
              ))}
            </div>
            {ticket.status !== 'closed' && (
              <>
                <Input.TextArea
                  rows={4}
                  maxLength={2000}
                  value={draft}
                  placeholder={t('pages.sales.tickets.replyHint')}
                  onChange={(e) => setDraft(e.target.value)}
                />
                <Button
                  type="primary"
                  style={{ marginTop: 8 }}
                  loading={replying}
                  disabled={draft.trim() === ''}
                  onClick={onReply}
                >
                  {t('pages.sales.tickets.send')}
                </Button>
              </>
            )}
          </>
        )}
      </Drawer>
    </>
  );
}
