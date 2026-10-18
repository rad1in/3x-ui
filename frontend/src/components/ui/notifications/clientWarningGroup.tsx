import { TeamOutlined } from '@ant-design/icons';
import type { NotificationGroupConfig } from './types';

// Thresholds live in General settings (quotaWarnPercent, expireDiff), shared by every channel.
export const CLIENT_WARNING_GROUP: NotificationGroupConfig = {
  icon: <TeamOutlined />,
  title: 'eventGroupClients',
  events: [
    { key: 'client.quota_warning', label: 'eventClientQuotaWarning', settingKey: '' },
    { key: 'client.expiry_warning', label: 'eventClientExpiryWarning', settingKey: '' },
    { key: 'report.weekly', label: 'eventWeeklyReport', settingKey: '' },
  ],
};
