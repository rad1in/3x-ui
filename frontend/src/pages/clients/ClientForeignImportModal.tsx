import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQueryClient } from '@tanstack/react-query';
import { Alert, Button, Input, Modal, Select, Space, Upload, message } from 'antd';
import { UploadOutlined } from '@ant-design/icons';
import { z } from 'zod';

import { HttpUtil } from '@/utils';
import { parseMsg } from '@/utils/zodValidate';
import { formatInboundLabel } from '@/lib/inbounds/label';
import { keys } from '@/api/queryKeys';
import type { InboundOption } from '@/hooks/useClients';

type ForeignSource = 'marzban' | 'marzneshin' | 'hiddify';

const ForeignImportResultSchema = z.object({
  created: z.number(),
  skipped: z.array(z.object({ email: z.string(), reason: z.string() })).optional(),
  warnings: z.array(z.object({ email: z.string(), message: z.string() })).optional(),
});
type ForeignImportResult = z.infer<typeof ForeignImportResultSchema>;

interface ClientForeignImportModalProps {
  open: boolean;
  inbounds: InboundOption[];
  onOpenChange: (open: boolean) => void;
}

export default function ClientForeignImportModal({
  open,
  inbounds,
  onOpenChange,
}: ClientForeignImportModalProps) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [messageApi, messageContextHolder] = message.useMessage();
  const [source, setSource] = useState<ForeignSource>('marzban');
  const [inboundIds, setInboundIds] = useState<number[]>([]);
  const [data, setData] = useState('');
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<ForeignImportResult | null>(null);

  async function onImport() {
    setBusy(true);
    try {
      const raw = await HttpUtil.post(
        '/panel/api/clients/importForeign',
        { source, data, inboundIds },
        { headers: { 'Content-Type': 'application/json' } },
      );
      const msg = parseMsg(raw, ForeignImportResultSchema, 'clients/importForeign');
      if (msg?.success && msg.obj) {
        setResult(msg.obj);
        messageApi.success(t('pages.clients.foreignImport.done', { count: msg.obj.created }));
        void queryClient.invalidateQueries({ queryKey: keys.clients.root() });
        void queryClient.invalidateQueries({ queryKey: keys.inbounds.root() });
      }
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal
      open={open}
      title={t('pages.clients.foreignImport.title')}
      footer={null}
      width={620}
      onCancel={() => onOpenChange(false)}
      destroyOnHidden
    >
      {messageContextHolder}
      <Space direction="vertical" size={10} style={{ width: '100%' }}>
        <Select
          style={{ width: '100%' }}
          value={source}
          options={[
            { value: 'marzban', label: t('pages.clients.foreignImport.source.marzban') },
            { value: 'marzneshin', label: t('pages.clients.foreignImport.source.marzneshin') },
            { value: 'hiddify', label: t('pages.clients.foreignImport.source.hiddify') },
          ]}
          onChange={setSource}
        />
        <Select
          mode="multiple"
          style={{ width: '100%' }}
          placeholder={t('pages.clients.foreignImport.inbounds')}
          value={inboundIds}
          options={inbounds.map((ib) => ({
            value: ib.id,
            label: formatInboundLabel(ib.tag, ib.remark) || String(ib.id),
          }))}
          onChange={setInboundIds}
        />
        <Upload
          accept=".json,application/json"
          maxCount={1}
          showUploadList={false}
          beforeUpload={(file) => {
            void file.text().then(setData);
            return false;
          }}
        >
          <Button icon={<UploadOutlined />}>{t('pages.clients.foreignImport.chooseFile')}</Button>
        </Upload>
        <Input.TextArea
          rows={6}
          value={data}
          placeholder={t('pages.clients.foreignImport.paste')}
          onChange={(e) => setData(e.target.value)}
        />
        <Alert type="info" showIcon message={t('pages.clients.foreignImport.note')} />
        <Button
          type="primary"
          loading={busy}
          disabled={data.trim() === '' || inboundIds.length === 0}
          onClick={onImport}
        >
          {t('pages.clients.foreignImport.run')}
        </Button>
        {result && (
          <Space direction="vertical" size={4}>
            {(result.skipped ?? []).map((s) => (
              <Alert key={`s-${s.email}`} type="warning" message={`${s.email}: ${s.reason}`} />
            ))}
            {(result.warnings ?? []).map((w) => (
              <Alert key={`w-${w.email}`} type="info" message={`${w.email}: ${w.message}`} />
            ))}
          </Space>
        )}
      </Space>
    </Modal>
  );
}
