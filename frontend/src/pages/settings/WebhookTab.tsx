import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Input, Space, Switch, Typography } from 'antd';
import { SendOutlined } from '@ant-design/icons';
import { HttpUtil } from '@/utils';
import type { AllSetting } from '@/models/setting';
import { SettingListItem } from '@/components/ui';
import { NotificationLayout } from '@/components/ui/notifications/NotificationLayout';
import { NotificationGroup } from '@/components/ui/notifications/NotificationGroup';
import SecretInput from './SecretInput';
import { EVENT_GROUPS } from './eventGroups';

interface WebhookTabProps {
  allSetting: AllSetting;
  updateSetting: (patch: Partial<AllSetting>) => void;
}

interface WebhookTestResult {
  success: boolean;
  msg: string;
}

export default function WebhookTab({ allSetting, updateSetting }: WebhookTabProps) {
  const { t } = useTranslation();
  const [testLoading, setTestLoading] = useState(false);
  const [testResult, setTestResult] = useState<WebhookTestResult | null>(null);

  const selected = (allSetting.webhookEvents || '')
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean);

  function setEvents(next: string[]) {
    updateSetting({ webhookEvents: next.join(',') });
  }

  function toggle(key: string) {
    setEvents(selected.includes(key) ? selected.filter((e) => e !== key) : [...selected, key]);
  }

  function toggleAll(keys: string[]) {
    const allSelected = keys.every((k) => selected.includes(k));
    setEvents(
      allSelected
        ? selected.filter((k) => !keys.includes(k))
        : [...new Set([...selected, ...keys])],
    );
  }

  async function handleTest() {
    setTestLoading(true);
    setTestResult(null);
    try {
      setTestResult((await HttpUtil.post('/panel/api/setting/testWebhook')) as WebhookTestResult);
    } catch (e: unknown) {
      setTestResult({
        success: false,
        msg: e instanceof Error ? e.message : t('pages.settings.requestFailed'),
      });
    } finally {
      setTestLoading(false);
    }
  }

  return (
    <>
      <SettingListItem
        paddings="small"
        title={t('pages.settings.webhookEnable')}
        description={t('pages.settings.webhookEnableDesc')}
      >
        <Switch
          checked={allSetting.webhookEnable}
          onChange={(v) => updateSetting({ webhookEnable: v })}
        />
      </SettingListItem>

      <SettingListItem
        paddings="small"
        title={t('pages.settings.webhookUrls')}
        description={t('pages.settings.webhookUrlsDesc')}
      >
        <Input.TextArea
          value={allSetting.webhookUrls}
          autoSize={{ minRows: 2, maxRows: 6 }}
          placeholder="https://example.com/hooks/3x-ui"
          onChange={(e) => updateSetting({ webhookUrls: e.target.value })}
          style={{ direction: 'ltr' }}
          aria-label={t('pages.settings.webhookUrls')}
        />
      </SettingListItem>

      <SettingListItem
        paddings="small"
        title={t('pages.settings.webhookSecret')}
        description={
          allSetting.hasWebhookSecret && !allSetting.clearWebhookSecret
            ? t('pages.settings.webhookSecretConfigured')
            : t('pages.settings.webhookSecretDesc')
        }
      >
        <SecretInput
          value={allSetting.webhookSecret}
          configured={allSetting.hasWebhookSecret}
          clearArmed={allSetting.clearWebhookSecret}
          placeholder="••••••••"
          onChange={(v) => updateSetting({ webhookSecret: v })}
          onClearArmedChange={(armed) => updateSetting({ clearWebhookSecret: armed })}
        />
      </SettingListItem>

      <SettingListItem
        paddings="small"
        title={t('pages.settings.webhookEvents')}
        description={t('pages.settings.webhookEventsDesc')}
      >
        <NotificationLayout>
          {EVENT_GROUPS.map((group) => (
            <NotificationGroup
              key={group.title}
              config={group}
              selected={selected}
              onToggle={toggle}
              onToggleAll={toggleAll}
              allSetting={allSetting}
              updateSetting={updateSetting}
            />
          ))}
        </NotificationLayout>
      </SettingListItem>

      <Space orientation="vertical" size={8} style={{ width: '100%', marginTop: 16 }}>
        <Typography.Text type="secondary">{t('pages.settings.webhookTestDesc')}</Typography.Text>
        <Button
          type="primary"
          icon={<SendOutlined />}
          loading={testLoading}
          disabled={!allSetting.webhookUrls.trim()}
          onClick={handleTest}
        >
          {t('pages.settings.testWebhook')}
        </Button>
        {testResult && (
          <Alert
            type={testResult.success ? 'success' : 'error'}
            title={testResult.msg}
            showIcon
            closable={{ onClose: () => setTestResult(null) }}
          />
        )}
      </Space>
    </>
  );
}
