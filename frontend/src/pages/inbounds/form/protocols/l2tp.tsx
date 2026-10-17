import { useTranslation } from 'react-i18next';
import { Alert, Input, Select } from 'antd';

import { FormField } from '@/components/form/rhf';

export default function L2TPFields() {
  const { t } = useTranslation();
  return (
    <>
      <Alert className="mb-12" type="info" showIcon title={t('pages.inbounds.form.l2tp.help')} />
      <FormField
        label={t('pages.inbounds.form.l2tp.psk')}
        tooltip={t('pages.inbounds.form.l2tp.pskDesc')}
        name={['settings', 'psk']}
      >
        <Input dir="ltr" />
      </FormField>
      <FormField
        label={t('pages.inbounds.form.ikev2.pool')}
        tooltip={t('pages.inbounds.form.l2tp.poolDesc')}
        name={['settings', 'pool']}
      >
        <Input placeholder="10.90.0.0/24" dir="ltr" />
      </FormField>
      <FormField label={t('pages.inbounds.form.openvpn.dns')} name={['settings', 'dns']}>
        <Select mode="tags" tokenSeparators={[',', ' ']} open={false} />
      </FormField>
    </>
  );
}
