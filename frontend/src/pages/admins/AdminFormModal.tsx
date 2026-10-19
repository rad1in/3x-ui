import { useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Form, Input, InputNumber, Modal, Select } from 'antd';
import { FormProvider, useWatch } from 'react-hook-form';

import { FormField, useZodForm } from '@/components/form/rhf';
import type { AdminView } from '@/generated/zod';
import { ADMIN_ROLES, type AdminRole } from '@/lib/adminRole';
import { adminFormSchema, type AdminFormValues } from '@/schemas/admin';

function initialValues(admin: AdminView | null): AdminFormValues {
  return {
    username: admin?.username ?? '',
    password: '',
    role: (admin?.role as AdminRole | undefined) ?? 'operator',
    inboundIds: admin?.inboundIds ?? [],
    groups: admin?.groups ?? [],
    tgChatId: admin?.tgChatId ?? 0,
  };
}

interface AdminFormModalProps {
  open: boolean;
  admin: AdminView | null;
  isSelf: boolean;
  saving: boolean;
  inboundOptions: { value: number; label: string }[];
  groupOptions: string[];
  onClose: () => void;
  onSubmit: (values: AdminFormValues) => void;
}

export default function AdminFormModal({
  open,
  admin,
  isSelf,
  saving,
  inboundOptions,
  groupOptions,
  onClose,
  onSubmit,
}: AdminFormModalProps) {
  const { t } = useTranslation();
  const schema = useMemo(() => adminFormSchema(admin === null), [admin]);
  const methods = useZodForm<AdminFormValues>(schema, { defaultValues: initialValues(admin) });
  const role = useWatch({ control: methods.control, name: 'role' });

  useEffect(() => {
    if (open) methods.reset(initialValues(admin));
  }, [open, admin, methods]);

  return (
    <Modal
      open={open}
      title={admin ? t('pages.admins.edit') : t('pages.admins.add')}
      okText={admin ? t('save') : t('create')}
      cancelText={t('close')}
      confirmLoading={saving}
      mask={{ closable: false }}
      width={560}
      onOk={methods.handleSubmit(onSubmit)}
      onCancel={onClose}
    >
      <FormProvider {...methods}>
        <Form layout="vertical">
          <FormField label={t('username')} name="username" required>
            <Input maxLength={64} autoComplete="off" />
          </FormField>
          <FormField
            label={t('password')}
            name="password"
            required={!admin}
            extra={admin ? t('pages.admins.passwordKeep') : undefined}
          >
            <Input.Password autoComplete="new-password" />
          </FormField>
          <FormField
            label={t('pages.admins.role')}
            name="role"
            extra={t(`pages.admins.roleHint.${role}`)}
          >
            <Select
              disabled={isSelf}
              options={ADMIN_ROLES.map((r) => ({ value: r, label: t(`pages.admins.roles.${r}`) }))}
            />
          </FormField>
          {role === 'operator' && (
            <>
              <FormField label={t('pages.admins.scopeInbounds')} name="inboundIds">
                <Select
                  mode="multiple"
                  disabled={isSelf}
                  options={inboundOptions}
                  optionFilterProp="label"
                />
              </FormField>
              <FormField label={t('pages.admins.scopeGroups')} name="groups">
                <Select
                  mode="tags"
                  disabled={isSelf}
                  options={groupOptions.map((g) => ({ value: g, label: g }))}
                />
              </FormField>
            </>
          )}
          <FormField
            label={t('pages.admins.tgChatId')}
            name="tgChatId"
            extra={t('pages.admins.tgChatIdHint')}
          >
            <InputNumber min={0} precision={0} style={{ width: '100%' }} />
          </FormField>
          {isSelf && <Alert type="info" showIcon title={t('pages.admins.selfNote')} />}
        </Form>
      </FormProvider>
    </Modal>
  );
}
