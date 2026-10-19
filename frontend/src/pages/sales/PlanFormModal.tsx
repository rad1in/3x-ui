import { useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Empty, Form, Input, InputNumber, Modal, Select, Switch } from 'antd';
import { FormProvider, useWatch } from 'react-hook-form';

import { FormField, useZodForm } from '@/components/form/rhf';
import { CURRENCIES, currencyDecimals } from '@/lib/money';
import { planFormSchema, type PlanFormValues } from '@/schemas/sales';

interface PlanFormModalProps {
  open: boolean;
  initial: PlanFormValues;
  templates: { id?: number; name: string }[];
  saving: boolean;
  onClose: () => void;
  onSubmit: (values: PlanFormValues) => void;
}

export default function PlanFormModal({
  open,
  initial,
  templates,
  saving,
  onClose,
  onSubmit,
}: PlanFormModalProps) {
  const { t } = useTranslation();
  const methods = useZodForm<PlanFormValues>(planFormSchema, { defaultValues: initial });
  const currency = useWatch({ control: methods.control, name: 'currency' });
  const templateOptions = useMemo(
    () => templates.flatMap((tpl) => (tpl.id == null ? [] : [{ value: tpl.id, label: tpl.name }])),
    [templates],
  );

  useEffect(() => {
    if (open) methods.reset(initial);
  }, [open, initial, methods]);

  return (
    <Modal
      open={open}
      title={initial.id ? t('pages.sales.plans.edit') : t('pages.sales.plans.add')}
      okText={t('save')}
      cancelText={t('close')}
      confirmLoading={saving}
      mask={{ closable: false }}
      width={560}
      onOk={methods.handleSubmit(onSubmit)}
      onCancel={onClose}
    >
      <FormProvider {...methods}>
        <Form layout="vertical">
          <FormField label={t('pages.sales.plans.name')} name="name" required>
            <Input maxLength={80} />
          </FormField>
          <FormField label={t('pages.sales.plans.details')} name="description">
            <Input.TextArea rows={2} maxLength={300} />
          </FormField>
          <div className="sales-row">
            <FormField label={t('pages.sales.plans.price')} name="price" required>
              <InputNumber
                min={0}
                precision={currencyDecimals(currency)}
                style={{ width: '100%' }}
              />
            </FormField>
            <FormField label={t('pages.sales.plans.currency')} name="currency">
              <Select options={CURRENCIES.map((c) => ({ value: c, label: c }))} />
            </FormField>
          </div>
          <div className="sales-row">
            <FormField label={t('pages.sales.plans.quotaGb')} name="totalGb">
              <InputNumber min={0} style={{ width: '100%' }} />
            </FormField>
            <FormField label={t('pages.sales.plans.durationDays')} name="durationDays">
              <InputNumber min={0} precision={0} style={{ width: '100%' }} />
            </FormField>
            <FormField label={t('pages.clients.limitIp')} name="limitIp">
              <InputNumber min={0} precision={0} style={{ width: '100%' }} />
            </FormField>
          </div>
          <FormField
            label={t('pages.sales.plans.template')}
            name="templateId"
            required
            extra={t('pages.sales.plans.templateHint')}
          >
            <Select
              options={templateOptions}
              notFoundContent={<Empty description={t('pages.clients.templates.noTemplates')} />}
            />
          </FormField>
          <div className="sales-row">
            <FormField label={t('pages.sales.plans.sortOrder')} name="sortOrder">
              <InputNumber precision={0} style={{ width: '100%' }} />
            </FormField>
            <FormField label={t('pages.sales.plans.enabled')} name="enabled" valueProp="checked">
              <Switch />
            </FormField>
          </div>
        </Form>
      </FormProvider>
    </Modal>
  );
}
