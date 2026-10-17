import { useTranslation } from 'react-i18next';
import { Button } from 'antd';
import { PlusOutlined, SearchOutlined, TeamOutlined } from '@ant-design/icons';

import EmptyState from '@/components/feedback/EmptyState';
import GettingStarted from '@/components/feedback/GettingStarted';

export interface ClientsEmptyStateProps {
  hasAnyClient: boolean;
  hasInbound: boolean;
  onAdd: () => void;
  onClear: () => void;
}

export default function ClientsEmptyState({
  hasAnyClient,
  hasInbound,
  onAdd,
  onClear,
}: ClientsEmptyStateProps) {
  const { t } = useTranslation();
  if (hasAnyClient) {
    return (
      <EmptyState
        icon={<SearchOutlined />}
        title={t('emptyState.noMatches')}
        action={<Button onClick={onClear}>{t('clear')}</Button>}
      />
    );
  }
  return (
    <EmptyState
      icon={<TeamOutlined />}
      title={t('emptyState.clientsTitle')}
      description={t('emptyState.clientsDesc')}
      action={
        <Button type="primary" icon={<PlusOutlined />} onClick={onAdd}>
          {t('pages.clients.addClients')}
        </Button>
      }
    >
      <GettingStarted hasInbound={hasInbound} hasClient={false} />
    </EmptyState>
  );
}
