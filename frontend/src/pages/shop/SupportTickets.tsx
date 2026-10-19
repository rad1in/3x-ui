import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Card, Input, List, Space, Tag, Typography } from 'antd';

import type { Ticket, TicketDetail } from '@/generated/zod';
import { errorKey, shopApi, type TicketAuth } from '@/lib/shopApi';

interface SupportTicketsProps {
  // An order token the customer already holds; without it they enter the
  // username and subscription id.
  orderToken: string;
}

export default function SupportTickets({ orderToken }: SupportTicketsProps) {
  const { t } = useTranslation();
  const [email, setEmail] = useState('');
  const [proof, setProof] = useState('');
  const [auth, setAuth] = useState<TicketAuth | null>(null);
  const [tickets, setTickets] = useState<Ticket[]>([]);
  const [current, setCurrent] = useState<TicketDetail | null>(null);
  const [subject, setSubject] = useState('');
  const [body, setBody] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  const fail = (code: string) => setError(errorKey(code));

  const load = useCallback(async (a: TicketAuth) => {
    setBusy(true);
    setError('');
    const res = await shopApi.tickets(a);
    setBusy(false);
    if (res.ok) {
      setAuth(a);
      setTickets(res.data);
    } else {
      setError(errorKey(res.code));
    }
  }, []);

  function signIn() {
    void load(orderToken ? { order: orderToken } : { email: email.trim(), proof: proof.trim() });
  }

  async function openTicket(id: number) {
    if (!auth) return;
    setError('');
    const res = await shopApi.ticket(auth, id);
    if (res.ok) setCurrent(res.data);
    else fail(res.code);
  }

  async function send() {
    if (!auth) return;
    setBusy(true);
    setError('');
    const res = current
      ? await shopApi.replyTicket(auth, current.ticket.id, body)
      : await shopApi.openTicket(auth, subject, body);
    setBusy(false);
    if (!res.ok) {
      fail(res.code);
      return;
    }
    setCurrent(res.data);
    setSubject('');
    setBody('');
    void load(auth);
  }

  return (
    <Card size="small" title={t('shop.tickets.title')} style={{ marginTop: 16 }}>
      <Space direction="vertical" size={12} style={{ width: '100%' }}>
        {error && <Alert type="error" showIcon title={t(error)} />}
        {!auth && (
          <>
            {!orderToken && (
              <>
                <Input
                  dir="ltr"
                  value={email}
                  placeholder={t('shop.yourUsername')}
                  onChange={(e) => setEmail(e.target.value)}
                />
                <Input
                  dir="ltr"
                  value={proof}
                  placeholder={t('shop.subscriptionId')}
                  onChange={(e) => setProof(e.target.value)}
                />
              </>
            )}
            <Button type="primary" loading={busy} onClick={signIn}>
              {t('shop.tickets.show')}
            </Button>
          </>
        )}
        {auth && !current && (
          <>
            <List
              size="small"
              locale={{ emptyText: t('shop.tickets.empty') }}
              dataSource={tickets}
              renderItem={(tk) => (
                <List.Item>
                  <Typography.Link onClick={() => void openTicket(tk.id)}>
                    <span dir="auto">{tk.subject}</span>
                  </Typography.Link>
                  <Tag>{t(`shop.tickets.statuses.${tk.status}`)}</Tag>
                </List.Item>
              )}
            />
            <Input
              value={subject}
              maxLength={120}
              placeholder={t('shop.tickets.subject')}
              onChange={(e) => setSubject(e.target.value)}
            />
          </>
        )}
        {current && (
          <>
            <Typography.Text strong dir="auto">
              {current.ticket.subject}
            </Typography.Text>
            <div className="shop-ticket-thread">
              {current.messages.map((m) => (
                <div
                  key={m.id}
                  dir="auto"
                  className={
                    m.author === 'admin' ? 'shop-ticket-message is-admin' : 'shop-ticket-message'
                  }
                >
                  {m.body}
                </div>
              ))}
            </div>
          </>
        )}
        {auth && (!current || current.ticket.status !== 'closed') && (
          <>
            <Input.TextArea
              rows={3}
              maxLength={2000}
              value={body}
              placeholder={t('shop.tickets.message')}
              onChange={(e) => setBody(e.target.value)}
            />
            <Space>
              <Button
                type="primary"
                loading={busy}
                disabled={body.trim() === '' || (!current && subject.trim() === '')}
                onClick={() => void send()}
              >
                {t('shop.tickets.send')}
              </Button>
              {current && (
                <Button onClick={() => setCurrent(null)}>{t('shop.tickets.back')}</Button>
              )}
            </Space>
          </>
        )}
        {current?.ticket.status === 'closed' && (
          <Alert type="info" showIcon title={t('shop.tickets.closed')} />
        )}
      </Space>
    </Card>
  );
}
