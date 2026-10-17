import { useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Col, Divider, Form, Input, InputNumber, Modal, Row, Switch } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import { FormProvider } from 'react-hook-form';
import dayjs from 'dayjs';

import { FormField, useZodForm } from '@/components/form/rhf';
import DateTimePicker from '@/components/form/DateTimePicker';
import type { HostedPanelView } from '@/generated/zod';
import {
  HOSTED_PASSWORD_MIN,
  bytesToGb,
  hostedPanelFormSchema,
  type HostedPanelFormValues,
} from '@/schemas/hostedPanel';
import { RandomUtil } from '@/utils';
import type { HostedPortSuggestion } from './panelState';

function initialValues(
  panel: HostedPanelView | null,
  suggested: HostedPortSuggestion,
): HostedPanelFormValues {
  if (!panel) {
    return {
      name: '',
      enable: true,
      username: '',
      password: RandomUtil.randomSeq(16),
      ...suggested,
      webBasePath: `/${RandomUtil.randomLowerAndNum(10)}/`,
      maxInbounds: 5,
      maxClients: 100,
      trafficGB: 0,
      expiresAt: 0,
    };
  }
  return {
    name: panel.name,
    enable: panel.enable,
    username: panel.username,
    password: '',
    webPort: panel.webPort,
    subPort: panel.subPort,
    webBasePath: panel.webBasePath,
    portMin: panel.portMin,
    portMax: panel.portMax,
    maxInbounds: panel.maxInbounds,
    maxClients: panel.maxClients,
    trafficGB: bytesToGb(panel.trafficBytes),
    expiresAt: panel.expiresAt,
  };
}

const msToDayjs = (ms: unknown) => (typeof ms === 'number' && ms > 0 ? dayjs(ms) : null);
const dayjsToMs = (d: unknown) => (dayjs.isDayjs(d) ? d.valueOf() : 0);
const fullWidth = { width: '100%' };

interface PanelFormModalProps {
  open: boolean;
  panel: HostedPanelView | null;
  suggested: HostedPortSuggestion;
  saving: boolean;
  onClose: () => void;
  onSubmit: (values: HostedPanelFormValues) => void;
}

export default function PanelFormModal({
  open,
  panel,
  suggested,
  saving,
  onClose,
  onSubmit,
}: PanelFormModalProps) {
  const { t } = useTranslation();
  const schema = useMemo(
    () => hostedPanelFormSchema(panel ? { username: panel.username } : null),
    [panel],
  );
  const methods = useZodForm<HostedPanelFormValues>(schema, {
    defaultValues: initialValues(panel, suggested),
  });

  useEffect(() => {
    if (open) methods.reset(initialValues(panel, suggested));
  }, [open, panel, suggested, methods]);

  const regenerate = (name: 'password' | 'webBasePath') =>
    methods.setValue(
      name,
      name === 'password' ? RandomUtil.randomSeq(16) : `/${RandomUtil.randomLowerAndNum(10)}/`,
      { shouldDirty: true, shouldValidate: true },
    );

  return (
    <Modal
      open={open}
      title={panel ? t('pages.hostedPanels.edit') : t('pages.hostedPanels.add')}
      okText={panel ? t('save') : t('create')}
      cancelText={t('close')}
      confirmLoading={saving}
      mask={{ closable: false }}
      width={680}
      onOk={methods.handleSubmit(onSubmit)}
      onCancel={onClose}
    >
      <FormProvider {...methods}>
        <Form layout="vertical" className="hosted-panel-form">
          <Row gutter={16}>
            <Col xs={24} sm={16}>
              <FormField label={t('pages.hostedPanels.name')} name="name" required>
                <Input maxLength={64} placeholder={t('pages.hostedPanels.namePlaceholder')} />
              </FormField>
            </Col>
            <Col xs={24} sm={8}>
              <FormField label={t('enable')} name="enable" valueProp="checked">
                <Switch />
              </FormField>
            </Col>
          </Row>

          <Divider titlePlacement="start" plain>
            {t('pages.hostedPanels.loginSection')}
          </Divider>
          <Row gutter={16}>
            <Col xs={24} sm={12}>
              <FormField label={t('username')} name="username" required>
                <Input maxLength={64} autoComplete="off" />
              </FormField>
            </Col>
            <Col xs={24} sm={12}>
              <FormField
                label={t('password')}
                name="password"
                required={!panel}
                extra={
                  panel
                    ? t('pages.hostedPanels.passwordKeep')
                    : t('pages.hostedPanels.passwordHint', { n: HOSTED_PASSWORD_MIN })
                }
              >
                <Input
                  autoComplete="new-password"
                  suffix={
                    <Button
                      type="text"
                      size="small"
                      icon={<ReloadOutlined />}
                      aria-label={t('regenerate')}
                      onClick={() => regenerate('password')}
                    />
                  }
                />
              </FormField>
            </Col>
          </Row>

          <Divider titlePlacement="start" plain>
            {t('pages.hostedPanels.addressSection')}
          </Divider>
          <Row gutter={16}>
            <Col xs={12} sm={6}>
              <FormField label={t('pages.hostedPanels.webPort')} name="webPort" required>
                <InputNumber min={1} max={65535} precision={0} style={fullWidth} />
              </FormField>
            </Col>
            <Col xs={12} sm={6}>
              <FormField label={t('pages.hostedPanels.subPort')} name="subPort" required>
                <InputNumber min={1} max={65535} precision={0} style={fullWidth} />
              </FormField>
            </Col>
            <Col xs={24} sm={12}>
              <FormField
                label={t('pages.hostedPanels.path')}
                name="webBasePath"
                required
                extra={panel ? t('pages.hostedPanels.addressRestart') : undefined}
              >
                <Input
                  suffix={
                    <Button
                      type="text"
                      size="small"
                      icon={<ReloadOutlined />}
                      aria-label={t('regenerate')}
                      onClick={() => regenerate('webBasePath')}
                    />
                  }
                />
              </FormField>
            </Col>
          </Row>

          <Divider titlePlacement="start" plain>
            {t('pages.hostedPanels.limitsSection')}
          </Divider>
          <Row gutter={16}>
            <Col xs={12} sm={6}>
              <FormField
                label={t('pages.hostedPanels.portMin')}
                name="portMin"
                required
                tooltip={t('pages.hostedPanels.portRangeHint')}
              >
                <InputNumber min={1} max={65535} precision={0} style={fullWidth} />
              </FormField>
            </Col>
            <Col xs={12} sm={6}>
              <FormField label={t('pages.hostedPanels.portMax')} name="portMax" required>
                <InputNumber min={1} max={65535} precision={0} style={fullWidth} />
              </FormField>
            </Col>
            <Col xs={24} sm={12}>
              <FormField
                label={t('pages.hostedPanels.traffic')}
                name="trafficGB"
                extra={t('pages.hostedPanels.zeroUnlimited')}
              >
                <InputNumber min={0} step={10} suffix="GB" style={fullWidth} />
              </FormField>
            </Col>
            <Col xs={12} sm={6}>
              <FormField label={t('pages.hostedPanels.maxInbounds')} name="maxInbounds">
                <InputNumber min={0} precision={0} style={fullWidth} />
              </FormField>
            </Col>
            <Col xs={12} sm={6}>
              <FormField label={t('pages.hostedPanels.maxClients')} name="maxClients">
                <InputNumber min={0} precision={0} style={fullWidth} />
              </FormField>
            </Col>
            <Col xs={24} sm={12}>
              <FormField
                label={t('pages.hostedPanels.expires')}
                name="expiresAt"
                extra={t('pages.hostedPanels.expiresHint')}
                transform={{ input: msToDayjs, output: dayjsToMs }}
              >
                <DateTimePicker value={null} onChange={() => {}} />
              </FormField>
            </Col>
          </Row>
        </Form>
      </FormProvider>
    </Modal>
  );
}
