import type { ReactNode } from 'react';

import './EmptyState.css';

export interface EmptyStateProps {
  icon: ReactNode;
  title: string;
  description?: string;
  action?: ReactNode;
  children?: ReactNode;
}

export default function EmptyState({
  icon,
  title,
  description,
  action,
  children,
}: EmptyStateProps) {
  return (
    <div className="empty-state" data-testid="empty-state">
      <div className="empty-state-icon" aria-hidden="true">
        {icon}
      </div>
      <div className="empty-state-title" dir="auto">
        {title}
      </div>
      {description && (
        <p className="empty-state-desc" dir="auto">
          {description}
        </p>
      )}
      {action && <div className="empty-state-action">{action}</div>}
      {children}
    </div>
  );
}
