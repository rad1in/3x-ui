import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Input, InputNumber, Space, Switch, Typography } from 'antd';
import { CloudUploadOutlined } from '@ant-design/icons';
import { HttpUtil } from '@/utils';
import { onNumber } from '@/utils/onNumber';
import type { AllSetting } from '@/models/setting';
import { SettingListItem } from '@/components/ui';
import { NotifyTimeField } from './NotifyTimeField';
import SecretInput from './SecretInput';

interface BackupTabProps {
  allSetting: AllSetting;
  updateSetting: (patch: Partial<AllSetting>) => void;
}

interface BackupResult {
  destination: string;
  name: string;
  pruned: number;
  error?: string;
}

interface BackupNowReply {
  success: boolean;
  msg?: string;
  obj?: BackupResult[];
}

export default function BackupTab({ allSetting, updateSetting }: BackupTabProps) {
  const { t } = useTranslation();
  const [running, setRunning] = useState(false);
  const [reply, setReply] = useState<BackupNowReply | null>(null);

  async function runNow() {
    setRunning(true);
    setReply(null);
    try {
      setReply((await HttpUtil.post('/panel/api/setting/backupNow')) as BackupNowReply);
    } catch (e: unknown) {
      setReply({
        success: false,
        msg: e instanceof Error ? e.message : t('pages.settings.requestFailed'),
      });
    } finally {
      setRunning(false);
    }
  }

  const ltr = { direction: 'ltr' as const };
  const text = (key: keyof AllSetting, label: string, desc?: string, placeholder?: string) => (
    <SettingListItem paddings="small" title={label} description={desc}>
      <Input
        value={String(allSetting[key] ?? '')}
        placeholder={placeholder}
        style={ltr}
        aria-label={label}
        onChange={(e) => updateSetting({ [key]: e.target.value } as Partial<AllSetting>)}
      />
    </SettingListItem>
  );

  return (
    <>
      <SettingListItem
        paddings="small"
        title={t('pages.settings.backupScheduleEnable')}
        description={t('pages.settings.backupScheduleEnableDesc')}
      >
        <Switch
          checked={allSetting.backupSchedule !== ''}
          onChange={(on) => updateSetting({ backupSchedule: on ? '@daily' : '' })}
        />
      </SettingListItem>
      {allSetting.backupSchedule !== '' && (
        <SettingListItem paddings="small" title={t('pages.settings.backupSchedule')}>
          <NotifyTimeField
            value={allSetting.backupSchedule}
            onChange={(v) => updateSetting({ backupSchedule: v })}
            ariaLabel={t('pages.settings.backupSchedule')}
          />
        </SettingListItem>
      )}
      <SettingListItem
        paddings="small"
        title={t('pages.settings.backupKeep')}
        description={t('pages.settings.backupKeepDesc')}
      >
        <InputNumber
          value={allSetting.backupKeep}
          min={0}
          style={{ width: '100%' }}
          onChange={onNumber((v) => updateSetting({ backupKeep: v }))}
        />
      </SettingListItem>

      <Typography.Title level={5} style={{ marginTop: 16 }}>
        {t('pages.settings.backupLocal')}
      </Typography.Title>
      {text(
        'backupLocalDir',
        t('pages.settings.backupLocalDir'),
        t('pages.settings.backupLocalDirDesc'),
        '/var/backups/eternal',
      )}

      <Typography.Title level={5} style={{ marginTop: 16 }}>
        {t('pages.settings.backupS3')}
      </Typography.Title>
      {text(
        'backupS3Endpoint',
        t('pages.settings.backupS3Endpoint'),
        t('pages.settings.backupS3EndpointDesc'),
        'https://s3.amazonaws.com',
      )}
      {text(
        'backupS3Region',
        t('pages.settings.backupS3Region'),
        t('pages.settings.backupS3RegionDesc'),
        'us-east-1',
      )}
      {text('backupS3Bucket', t('pages.settings.backupS3Bucket'))}
      {text('backupS3Prefix', t('pages.settings.backupS3Prefix'), undefined, 'eternal/')}
      {text('backupS3AccessKey', t('pages.settings.backupS3AccessKey'))}
      <SettingListItem paddings="small" title={t('pages.settings.backupS3SecretKey')}>
        <SecretInput
          value={allSetting.backupS3SecretKey}
          configured={allSetting.hasBackupS3Secret}
          clearArmed={allSetting.clearBackupS3Secret}
          placeholder="••••••••"
          onChange={(v) => updateSetting({ backupS3SecretKey: v })}
          onClearArmedChange={(armed) => updateSetting({ clearBackupS3Secret: armed })}
        />
      </SettingListItem>

      <Space orientation="vertical" size={8} style={{ width: '100%', marginTop: 16 }}>
        <Typography.Text type="secondary">{t('pages.settings.backupNowDesc')}</Typography.Text>
        <Button type="primary" icon={<CloudUploadOutlined />} loading={running} onClick={runNow}>
          {t('pages.settings.backupNow')}
        </Button>
        {reply && !reply.success && (
          <Alert type="error" title={reply.msg || t('pages.settings.requestFailed')} showIcon />
        )}
        {reply?.success &&
          (reply.obj ?? []).map((r) => (
            <Alert
              key={r.destination}
              type={r.error ? 'error' : 'success'}
              showIcon
              title={`${r.destination === 's3' ? 'S3' : t('pages.settings.backupLocal')}: ${r.error || r.name}`}
            />
          ))}
      </Space>
    </>
  );
}
