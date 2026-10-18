import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Table, Tag } from 'antd';
import type { ColumnsType } from 'antd/es/table';

import { HttpUtil, IntlUtil } from '@/utils';
import type { SubFetchInfo } from '@/generated/types';

interface SubFetchListProps {
  subId: string;
  datepicker: 'gregorian' | 'jalalian';
}

export default function SubFetchList({ subId, datepicker }: SubFetchListProps) {
  const { t } = useTranslation();
  const [rows, setRows] = useState<SubFetchInfo[]>([]);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      const msg = await HttpUtil.get<SubFetchInfo[]>(
        `/panel/api/clients/subFetches/${encodeURIComponent(subId)}`,
      );
      if (!cancelled) setRows(msg?.success && Array.isArray(msg.obj) ? msg.obj : []);
    })();
    return () => {
      cancelled = true;
    };
  }, [subId]);

  if (rows.length === 0) return null;

  const columns: ColumnsType<SubFetchInfo> = [
    {
      title: t('pages.clients.fetchApp'),
      dataIndex: 'userAgent',
      ellipsis: true,
      render: (ua: string, row) => (
        <>
          <Tag>{row.format}</Tag>
          <span title={ua}>{ua || '-'}</span>
        </>
      ),
    },
    {
      title: 'IP',
      dataIndex: 'ip',
      width: 130,
      render: (ip: string) => <span dir="ltr">{ip}</span>,
    },
    { title: t('pages.clients.fetchCount'), dataIndex: 'count', width: 70 },
    {
      title: t('pages.clients.fetchLast'),
      dataIndex: 'lastSeen',
      width: 160,
      render: (ms: number) => IntlUtil.formatDate(ms, datepicker),
    },
  ];

  return (
    <div className="sub-fetch-list">
      <div className="sub-fetch-title">{t('pages.clients.fetchHistory')}</div>
      <Table<SubFetchInfo>
        size="small"
        pagination={false}
        rowKey={(r) => `${r.ip}|${r.userAgent}|${r.format}`}
        columns={columns}
        dataSource={rows}
        scroll={{ x: 'max-content' }}
      />
    </div>
  );
}
