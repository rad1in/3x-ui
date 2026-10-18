import {
  CloudServerOutlined,
  DesktopOutlined,
  SafetyOutlined,
  TeamOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons';
import type { NotificationGroupConfig } from '@/components/ui/notifications/types';

// cpu.high and memory.high are left out: they are raw per-minute samples that
// each notifier filters by its own threshold, not alarms.
export const EVENT_GROUPS: NotificationGroupConfig[] = [
  {
    icon: <TeamOutlined />,
    title: 'eventGroupClients',
    events: [
      { key: 'client.created', label: 'eventClientCreated', settingKey: '' },
      { key: 'client.deleted', label: 'eventClientDeleted', settingKey: '' },
      { key: 'client.enabled', label: 'eventClientEnabled', settingKey: '' },
      { key: 'client.disabled', label: 'eventClientDisabled', settingKey: '' },
      { key: 'client.depleted', label: 'eventClientDepleted', settingKey: '' },
      { key: 'client.expired', label: 'eventClientExpired', settingKey: '' },
      { key: 'client.renewed', label: 'eventClientRenewed', settingKey: '' },
      { key: 'client.traffic_reset', label: 'eventClientTrafficReset', settingKey: '' },
      { key: 'client.quota_warning', label: 'eventClientQuotaWarning', settingKey: '' },
      { key: 'client.expiry_warning', label: 'eventClientExpiryWarning', settingKey: '' },
      { key: 'report.weekly', label: 'eventWeeklyReport', settingKey: '' },
      { key: 'reseller.credit_low', label: 'eventResellerCreditLow', settingKey: '' },
    ],
  },
  {
    icon: <CloudServerOutlined />,
    title: 'eventGroupOutbound',
    events: [
      { key: 'outbound.down', label: 'eventOutboundDown', settingKey: '' },
      { key: 'outbound.up', label: 'eventOutboundUp', settingKey: '' },
    ],
  },
  {
    icon: <ThunderboltOutlined />,
    title: 'eventGroupXray',
    events: [{ key: 'xray.crash', label: 'eventXrayCrash', settingKey: '' }],
  },
  {
    icon: <DesktopOutlined />,
    title: 'eventGroupNode',
    events: [
      { key: 'node.down', label: 'eventNodeDown', settingKey: '' },
      { key: 'node.up', label: 'eventNodeUp', settingKey: '' },
      { key: 'node.cap_reached', label: 'eventNodeCapReached', settingKey: '' },
    ],
  },
  {
    icon: <SafetyOutlined />,
    title: 'eventGroupSecurity',
    events: [{ key: 'login.attempt', label: 'eventLoginAttempt', settingKey: '' }],
  },
];
