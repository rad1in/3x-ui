import { useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Divider, Input, Space, Switch, Typography } from 'antd';
import { SendOutlined } from '@ant-design/icons';
import { HttpUtil } from '@/utils';
import type { AllSetting } from '@/models/setting';
import { SettingListItem } from '@/components/ui';
import { NotificationLayout } from '@/components/ui/notifications/NotificationLayout';
import { NotificationGroup } from '@/components/ui/notifications/NotificationGroup';
import SecretInput from './SecretInput';
import { EVENT_GROUPS } from './eventGroups';
import WeeklyReportSettings from './WeeklyReportSettings';

interface PushTabProps {
  allSetting: AllSetting;
  updateSetting: (patch: Partial<AllSetting>) => void;
}

interface TestResult {
  success: boolean;
  msg: string;
}

interface ChannelSectionProps {
  title: string;
  channel: 'ntfy' | 'gotify' | 'slack';
  enabled: boolean;
  events: string;
  canTest: boolean;
  allSetting: AllSetting;
  updateSetting: (patch: Partial<AllSetting>) => void;
  onEnabled: (v: boolean) => void;
  onEvents: (v: string) => void;
  children: ReactNode;
}

function ChannelSection({
  title,
  channel,
  enabled,
  events,
  canTest,
  allSetting,
  updateSetting,
  onEnabled,
  onEvents,
  children,
}: ChannelSectionProps) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState<TestResult | null>(null);
  const selected = events
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean);

  function toggle(key: string) {
    onEvents(
      (selected.includes(key) ? selected.filter((e) => e !== key) : [...selected, key]).join(','),
    );
  }

  function toggleAll(keys: string[]) {
    const allSelected = keys.every((k) => selected.includes(k));
    onEvents(
      (allSelected
        ? selected.filter((k) => !keys.includes(k))
        : [...new Set([...selected, ...keys])]
      ).join(','),
    );
  }

  async function handleTest() {
    setLoading(true);
    setResult(null);
    try {
      setResult((await HttpUtil.post('/panel/api/setting/testPush', { channel })) as TestResult);
    } catch (e: unknown) {
      setResult({
        success: false,
        msg: e instanceof Error ? e.message : t('pages.settings.requestFailed'),
      });
    } finally {
      setLoading(false);
    }
  }

  return (
    <>
      <Divider titlePlacement="start">{title}</Divider>
      <SettingListItem
        paddings="small"
        title={t('pages.settings.pushEnable')}
        description={t('pages.settings.pushEnableDesc')}
      >
        <Switch checked={enabled} onChange={onEnabled} />
      </SettingListItem>
      {children}
      <SettingListItem
        paddings="small"
        title={t('pages.settings.pushEvents')}
        description={t('pages.settings.pushEventsDesc')}
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
      <Space orientation="vertical" size={8} style={{ width: '100%', marginTop: 8 }}>
        <Typography.Text type="secondary">{t('pages.settings.pushTestDesc')}</Typography.Text>
        <Button
          type="primary"
          icon={<SendOutlined />}
          loading={loading}
          disabled={!canTest}
          onClick={handleTest}
        >
          {t('pages.settings.pushTest')}
        </Button>
        {result && (
          <Alert
            type={result.success ? 'success' : 'error'}
            title={result.msg}
            showIcon
            closable={{ onClose: () => setResult(null) }}
          />
        )}
      </Space>
    </>
  );
}

const ltr = { direction: 'ltr' } as const;

export default function PushTab({ allSetting: s, updateSetting }: PushTabProps) {
  const { t } = useTranslation();
  const common = { allSetting: s, updateSetting };
  return (
    <>
      <WeeklyReportSettings allSetting={s} updateSetting={updateSetting} />
      <ChannelSection
        {...common}
        title="ntfy"
        channel="ntfy"
        enabled={s.ntfyEnable}
        events={s.ntfyEvents}
        canTest={s.ntfyServer.trim() !== '' && s.ntfyTopic.trim() !== ''}
        onEnabled={(v) => updateSetting({ ntfyEnable: v })}
        onEvents={(v) => updateSetting({ ntfyEvents: v })}
      >
        <SettingListItem paddings="small" title={t('pages.settings.ntfyServer')}>
          <Input
            value={s.ntfyServer}
            placeholder="https://ntfy.sh"
            onChange={(e) => updateSetting({ ntfyServer: e.target.value })}
            style={ltr}
            aria-label={t('pages.settings.ntfyServer')}
          />
        </SettingListItem>
        <SettingListItem paddings="small" title={t('pages.settings.ntfyTopic')}>
          <Input
            value={s.ntfyTopic}
            onChange={(e) => updateSetting({ ntfyTopic: e.target.value })}
            style={ltr}
            aria-label={t('pages.settings.ntfyTopic')}
          />
        </SettingListItem>
        <SettingListItem
          paddings="small"
          title={t('pages.settings.ntfyToken')}
          description={
            s.hasNtfyToken && !s.clearNtfyToken
              ? t('pages.settings.pushSecretConfigured')
              : undefined
          }
        >
          <SecretInput
            value={s.ntfyToken}
            configured={s.hasNtfyToken}
            clearArmed={s.clearNtfyToken}
            placeholder="••••••••"
            onChange={(v) => updateSetting({ ntfyToken: v })}
            onClearArmedChange={(armed) => updateSetting({ clearNtfyToken: armed })}
          />
        </SettingListItem>
      </ChannelSection>

      <ChannelSection
        {...common}
        title="Gotify"
        channel="gotify"
        enabled={s.gotifyEnable}
        events={s.gotifyEvents}
        canTest={s.gotifyUrl.trim() !== '' && (s.hasGotifyToken || s.gotifyToken.trim() !== '')}
        onEnabled={(v) => updateSetting({ gotifyEnable: v })}
        onEvents={(v) => updateSetting({ gotifyEvents: v })}
      >
        <SettingListItem paddings="small" title={t('pages.settings.gotifyUrl')}>
          <Input
            value={s.gotifyUrl}
            placeholder="https://gotify.example.com"
            onChange={(e) => updateSetting({ gotifyUrl: e.target.value })}
            style={ltr}
            aria-label={t('pages.settings.gotifyUrl')}
          />
        </SettingListItem>
        <SettingListItem
          paddings="small"
          title={t('pages.settings.gotifyToken')}
          description={
            s.hasGotifyToken && !s.clearGotifyToken
              ? t('pages.settings.pushSecretConfigured')
              : undefined
          }
        >
          <SecretInput
            value={s.gotifyToken}
            configured={s.hasGotifyToken}
            clearArmed={s.clearGotifyToken}
            placeholder="••••••••"
            onChange={(v) => updateSetting({ gotifyToken: v })}
            onClearArmedChange={(armed) => updateSetting({ clearGotifyToken: armed })}
          />
        </SettingListItem>
      </ChannelSection>

      <ChannelSection
        {...common}
        title="Slack"
        channel="slack"
        enabled={s.slackEnable}
        events={s.slackEvents}
        canTest={s.hasSlackWebhookUrl || s.slackWebhookUrl.trim() !== ''}
        onEnabled={(v) => updateSetting({ slackEnable: v })}
        onEvents={(v) => updateSetting({ slackEvents: v })}
      >
        <SettingListItem
          paddings="small"
          title={t('pages.settings.slackWebhookUrl')}
          description={
            s.hasSlackWebhookUrl && !s.clearSlackWebhookUrl
              ? t('pages.settings.pushSecretConfigured')
              : undefined
          }
        >
          <SecretInput
            value={s.slackWebhookUrl}
            configured={s.hasSlackWebhookUrl}
            clearArmed={s.clearSlackWebhookUrl}
            placeholder="https://hooks.slack.com/services/…"
            onChange={(v) => updateSetting({ slackWebhookUrl: v })}
            onClearArmedChange={(armed) => updateSetting({ clearSlackWebhookUrl: armed })}
          />
        </SettingListItem>
      </ChannelSection>
    </>
  );
}
