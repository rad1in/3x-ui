import { useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Form, Input, InputNumber, Modal, Select, Switch } from 'antd';
import { FormProvider, useWatch } from 'react-hook-form';
import dayjs from 'dayjs';

import DateTimePicker from '@/components/form/DateTimePicker';
import { FormField, useZodForm } from '@/components/form/rhf';
import type { Plan } from '@/generated/zod';
import { CURRENCIES, currencyDecimals } from '@/lib/money';
import { discountFormSchema, type DiscountFormValues } from '@/schemas/sales';

const msToDayjs = (ms: unknown) => (typeof ms === 'number' && ms > 0 ? dayjs(ms) : null);
const dayjsToMs = (d: unknown) => (dayjs.isDayjs(d) ? d.valueOf() : 0);

interface DiscountFormModalProps {
  open: boolean;
  initial: DiscountFormValues;
  plans: Plan[];
  saving: boolean;
  onClose: () => void;
  onSubmit: (values: DiscountFormValues) => void;
}

export default function DiscountFormModal({
  open,
  initial,
  plans,
  saving,
  onClose,
  onSubmit,
}: DiscountFormModalProps) {
  const { t } = useTranslation();
  const methods = useZodForm<DiscountFormValues>(discountFormSchema, { defaultValues: initial });
  const kind = useWatch({ control: methods.control, name: 'kind' });
  const currency = useWatch({ control: methods.control, name: 'currency' });
  const planOptions = useMemo(() => plans.map((p) => ({ value: p.id, label: p.name })), [plans]);

  useEffect(() => {
    if (open) methods.reset(initial);
  }, [open, initial, methods]);

  return (
    <Modal
      open={open}
      title={initial.id ? t('pages.sales.discounts.edit') : t('pages.sales.discounts.add')}
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
          <FormField label={t('pages.sales.discounts.code')} name="code" required>
            <Input dir="ltr" maxLength={32} style={{ textTransform: 'uppercase' }} />
          </FormField>
          <div className="sales-row">
            <FormField label={t('pages.sales.discounts.kind')} name="kind">
              <Select
                options={[
                  { value: 'percent', label: t('pages.sales.discounts.percent') },
                  { value: 'fixed', label: t('pages.sales.discounts.fixed') },
                ]}
              />
            </FormField>
            <FormField label={t('pages.sales.discounts.value')} name="value" required>
              <InputNumber
                min={0}
                precision={kind === 'fixed' ? currencyDecimals(currency) : 0}
                addonAfter={kind === 'percent' ? '%' : undefined}
                style={{ width: '100%' }}
              />
            </FormField>
            {kind === 'fixed' && (
              <FormField label={t('pages.sales.plans.currency')} name="currency">
                <Select options={CURRENCIES.map((c) => ({ value: c, label: c }))} />
              </FormField>
            )}
          </div>
          <FormField
            label={t('pages.sales.discounts.plans')}
            name="planIds"
            extra={t('pages.sales.discounts.plansHint')}
          >
            <Select mode="multiple" options={planOptions} optionFilterProp="label" />
          </FormField>
          <div className="sales-row">
            <FormField
              label={t('pages.sales.discounts.maxUses')}
              name="maxUses"
              extra={t('pages.sales.discounts.maxUsesHint')}
            >
              <InputNumber min={0} precision={0} style={{ width: '100%' }} />
            </FormField>
            <FormField
              label={t('pages.sales.discounts.expiresAt')}
              name="expiresAt"
              transform={{ input: msToDayjs, output: dayjsToMs }}
            >
              <DateTimePicker value={null} onChange={() => {}} />
            </FormField>
          </div>
          <div className="sales-row">
            <FormField
              label={t('pages.sales.discounts.oncePerCustomer')}
              name="oncePerCustomer"
              valueProp="checked"
            >
              <Switch />
            </FormField>
            <FormField
              label={t('pages.sales.discounts.enabled')}
              name="enabled"
              valueProp="checked"
            >
              <Switch />
            </FormField>
          </div>
        </Form>
      </FormProvider>
    </Modal>
  );
}
