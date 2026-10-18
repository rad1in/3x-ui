import { useTranslation } from 'react-i18next';
import { Alert, Input, Select } from 'antd';

import { FormField } from '@/components/form/rhf';

export default function SSTPFields() {
  const { t } = useTranslation();
  return (
    <>
      <Alert className="mb-12" type="info" showIcon title={t('pages.inbounds.form.sstp.help')} />
      <FormField
        label={t('pages.inbounds.form.ikev2.serverAddress')}
        tooltip={t('pages.inbounds.form.ikev2.serverAddressDesc')}
        name={['settings', 'serverAddress']}
      >
        <Input placeholder="vpn.example.com" dir="ltr" />
      </FormField>
      <FormField
        label={t('pages.inbounds.form.ikev2.pool')}
        tooltip={t('pages.inbounds.form.sstp.poolDesc')}
        name={['settings', 'pool']}
      >
        <Input placeholder="10.91.0.0/24" dir="ltr" />
      </FormField>
      <FormField label={t('pages.inbounds.form.openvpn.dns')} name={['settings', 'dns']}>
        <Select mode="tags" tokenSeparators={[',', ' ']} open={false} />
      </FormField>
    </>
  );
}
