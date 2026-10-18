import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Form, Modal, Select } from 'antd';
import { FormProvider, useWatch } from 'react-hook-form';

import { FormField, useZodForm } from '@/components/form/rhf';
import type { AdminView, ClientTransferResult } from '@/generated/zod';
import { transferFormSchema, type TransferFormValues } from '@/schemas/admin';

const EMPTY: TransferFormValues = { toAdminId: 0, group: '', emails: [] };

interface TransferClientsModalProps {
  open: boolean;
  from: AdminView | null;
  operators: AdminView[];
  saving: boolean;
  result: ClientTransferResult | null;
  onClose: () => void;
  onSubmit: (values: TransferFormValues) => void;
}

export default function TransferClientsModal({
  open,
  from,
  operators,
  saving,
  result,
  onClose,
  onSubmit,
}: TransferClientsModalProps) {
  const { t } = useTranslation();
  const methods = useZodForm<TransferFormValues>(transferFormSchema, { defaultValues: EMPTY });
  const toAdminId = useWatch({ control: methods.control, name: 'toAdminId' });
  const target = operators.find((o) => o.id === toAdminId);

  useEffect(() => {
    if (open) methods.reset(EMPTY);
  }, [open, from, methods]);

  return (
    <Modal
      open={open}
      title={`${t('pages.admins.transfer.title')}: ${from?.username ?? ''}`}
      okText={t('pages.admins.transfer.submit')}
      cancelText={t('close')}
      confirmLoading={saving}
      mask={{ closable: false }}
      width={560}
      onOk={methods.handleSubmit(onSubmit)}
      onCancel={onClose}
    >
      <FormProvider {...methods}>
        <Form layout="vertical">
          <p className="admins-hint" dir="auto">
            {t('pages.admins.transfer.hint')}
          </p>
          <FormField label={t('pages.admins.transfer.target')} name="toAdminId" required>
            <Select
              options={operators.map((o) => ({ value: o.id, label: o.username }))}
              optionFilterProp="label"
            />
          </FormField>
          <FormField
            label={t('pages.admins.transfer.group')}
            name="group"
            extra={t('pages.admins.transfer.groupHint')}
          >
            <Select
              allowClear
              options={(target?.groups ?? []).map((g) => ({ value: g, label: g }))}
            />
          </FormField>
          <FormField
            label={t('pages.admins.transfer.emails')}
            name="emails"
            extra={t('pages.admins.transfer.emailsHint')}
          >
            <Select mode="tags" tokenSeparators={[',', ' ', '\n']} open={false} />
          </FormField>
        </Form>
      </FormProvider>
      {result && (
        <Alert
          type={result.skipped.length === 0 ? 'success' : 'warning'}
          showIcon
          title={t('pages.admins.transfer.moved', { count: result.moved.length })}
          description={
            result.skipped.length > 0 && (
              <ul className="admins-skipped">
                {result.skipped.map((s) => (
                  <li key={s.email} dir="auto">
                    {s.email}:{' '}
                    {t(`pages.admins.transfer.reasons.${s.reason}`, { defaultValue: s.reason })}
                  </li>
                ))}
              </ul>
            )
          }
        />
      )}
    </Modal>
  );
}
