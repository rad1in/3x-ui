import { useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Checkbox, Form, Input, Modal, Space, Switch } from 'antd';
import { DeleteOutlined, UploadOutlined } from '@ant-design/icons';
import { FormProvider, useWatch } from 'react-hook-form';
import dayjs from 'dayjs';

import { FormField, useZodForm } from '@/components/form/rhf';
import DateTimePicker from '@/components/form/DateTimePicker';
import type { SponsorRecord } from '@/generated/zod';
import { BRAND_LOGO_MAX_BYTES, readBrandLogo } from '@/lib/brand/brand';
import { SPONSOR_SLOTS, SponsorFormSchema, type SponsorFormValues } from '@/schemas/sponsor';
import { getMessage } from '@/utils/messageBus';

export const SLOT_LABEL_KEYS: Record<(typeof SPONSOR_SLOTS)[number], string> = {
  dashboard: 'pages.sponsors.admin.slotDashboard',
  login: 'pages.sponsors.admin.slotLogin',
  page: 'pages.sponsors.admin.slotPage',
};

function initialValues(record: SponsorRecord | null): SponsorFormValues {
  return {
    name: record?.name ?? '',
    enable: record?.enable ?? true,
    slots: (record?.slots ?? ['dashboard', 'login']).filter(
      (s): s is SponsorFormValues['slots'][number] =>
        (SPONSOR_SLOTS as readonly string[]).includes(s),
    ),
    from: record?.from ?? 0,
    until: record?.until ?? 0,
    title: record?.title ?? '',
    text: record?.text ?? '',
    link: record?.link ?? 'https://',
    logo: record?.logo ?? '',
  };
}

const msToDayjs = (ms: unknown) => (typeof ms === 'number' && ms > 0 ? dayjs(ms) : null);
const dayjsToMs = (d: unknown) => (dayjs.isDayjs(d) ? d.valueOf() : 0);

interface SponsorFormModalProps {
  open: boolean;
  record: SponsorRecord | null;
  saving: boolean;
  onClose: () => void;
  onSubmit: (values: SponsorFormValues) => void;
}

export default function SponsorFormModal({
  open,
  record,
  saving,
  onClose,
  onSubmit,
}: SponsorFormModalProps) {
  const { t } = useTranslation();
  const methods = useZodForm<SponsorFormValues>(SponsorFormSchema, {
    defaultValues: initialValues(record),
  });
  const fileInput = useRef<HTMLInputElement>(null);
  const logo = useWatch({ control: methods.control, name: 'logo' });

  useEffect(() => {
    if (open) methods.reset(initialValues(record));
  }, [open, record, methods]);

  async function onLogoPicked(file: File | undefined) {
    if (!file) return;
    const result = await readBrandLogo(file);
    if (result.ok) {
      methods.setValue('logo', result.dataUrl, { shouldDirty: true });
      return;
    }
    getMessage().error(
      result.reason === 'type'
        ? t('pages.settings.brand.logoBadType')
        : t('pages.settings.brand.logoTooLarge', { kb: BRAND_LOGO_MAX_BYTES / 1024 }),
    );
  }

  return (
    <Modal
      open={open}
      title={record ? t('pages.sponsors.admin.edit') : t('pages.sponsors.admin.add')}
      okText={t('save')}
      cancelText={t('close')}
      confirmLoading={saving}
      mask={{ closable: false }}
      width={600}
      onOk={methods.handleSubmit(onSubmit)}
      onCancel={onClose}
    >
      <FormProvider {...methods}>
        <Form layout="vertical">
          <FormField label={t('pages.sponsors.admin.name')} name="name" required>
            <Input maxLength={64} placeholder="Acme VPS" />
          </FormField>
          <FormField label={t('pages.sponsors.admin.headline')} name="title">
            <Input maxLength={120} dir="auto" />
          </FormField>
          <FormField label={t('pages.sponsors.admin.text')} name="text">
            <Input.TextArea
              maxLength={300}
              showCount
              autoSize={{ minRows: 2, maxRows: 4 }}
              dir="auto"
            />
          </FormField>
          <FormField label={t('pages.sponsors.admin.link')} name="link" required>
            <Input placeholder="https://" inputMode="url" />
          </FormField>
          <FormField label={t('pages.sponsors.admin.slots')} name="slots" required>
            <Checkbox.Group
              options={SPONSOR_SLOTS.map((slot) => ({
                value: slot,
                label: t(SLOT_LABEL_KEYS[slot]),
              }))}
            />
          </FormField>
          <Space wrap size="large" align="start">
            <FormField
              label={t('pages.sponsors.admin.from')}
              name="from"
              transform={{ input: msToDayjs, output: dayjsToMs }}
            >
              <DateTimePicker value={null} onChange={() => {}} />
            </FormField>
            <FormField
              label={t('pages.sponsors.admin.until')}
              name="until"
              extra={t('pages.sponsors.admin.untilHint')}
              transform={{ input: msToDayjs, output: dayjsToMs }}
            >
              <DateTimePicker value={null} onChange={() => {}} />
            </FormField>
          </Space>
          <Form.Item label={t('pages.settings.brand.logo')}>
            <Space>
              {logo && <img className="sponsor-admin-logo" src={logo} alt="" />}
              <input
                ref={fileInput}
                type="file"
                accept="image/png,image/jpeg,image/webp"
                hidden
                onChange={(e) => {
                  void onLogoPicked(e.target.files?.[0]);
                  e.target.value = '';
                }}
              />
              <Button icon={<UploadOutlined />} onClick={() => fileInput.current?.click()}>
                {t('pages.settings.brand.logoUpload')}
              </Button>
              {logo && (
                <Button
                  danger
                  icon={<DeleteOutlined />}
                  aria-label={t('remove')}
                  onClick={() => methods.setValue('logo', '', { shouldDirty: true })}
                />
              )}
            </Space>
          </Form.Item>
          <FormField label={t('enable')} name="enable" valueProp="checked">
            <Switch />
          </FormField>
        </Form>
      </FormProvider>
    </Modal>
  );
}
