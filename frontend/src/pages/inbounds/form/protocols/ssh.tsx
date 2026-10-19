import { useTranslation } from 'react-i18next';
import { Alert, Input, Switch } from 'antd';

import { FormField } from '@/components/form/rhf';

export default function SSHFields() {
  const { t } = useTranslation();
  return (
    <>
      <Alert className="mb-12" type="info" showIcon title={t('pages.inbounds.form.ssh.help')} />
      <FormField
        label={t('pages.inbounds.form.ssh.banner')}
        tooltip={t('pages.inbounds.form.ssh.bannerDesc')}
        name={['settings', 'banner']}
      >
        <Input.TextArea rows={2} />
      </FormField>
      <FormField
        label={t('pages.inbounds.form.ssh.allowLocal')}
        tooltip={t('pages.inbounds.form.ssh.allowLocalDesc')}
        name={['settings', 'allowLocal']}
        valueProp="checked"
      >
        <Switch />
      </FormField>
    </>
  );
}
