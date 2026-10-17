import { useTranslation } from 'react-i18next';
import { Alert, Input, Select } from 'antd';

import { FormField } from '@/components/form/rhf';

export default function OpenVPNFields() {
  const { t } = useTranslation();
  return (
    <>
      <Alert className="mb-12" type="info" showIcon title={t('pages.inbounds.form.openvpn.help')} />
      <FormField label={t('pages.inbounds.form.openvpn.transport')} name={['settings', 'proto']}>
        <Select
          options={[
            { value: 'udp', label: 'UDP' },
            { value: 'tcp', label: 'TCP' },
          ]}
        />
      </FormField>
      <FormField
        label={t('pages.inbounds.form.openvpn.network')}
        tooltip={t('pages.inbounds.form.openvpn.networkDesc')}
        name={['settings', 'network']}
      >
        <Input placeholder="10.88.0.0/24" dir="ltr" />
      </FormField>
      <FormField label={t('pages.inbounds.form.openvpn.dns')} name={['settings', 'dns']}>
        <Select mode="tags" tokenSeparators={[',', ' ']} open={false} />
      </FormField>
    </>
  );
}
