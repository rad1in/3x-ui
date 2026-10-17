import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';
import { CheckOutlined } from '@ant-design/icons';

export interface GettingStartedProps {
  hasInbound: boolean;
  hasClient: boolean;
}

export default function GettingStarted({ hasInbound, hasClient }: GettingStartedProps) {
  const { t } = useTranslation();
  const steps = [
    { key: 'inbound', label: t('gettingStarted.step1'), done: hasInbound, to: '/inbounds' },
    { key: 'client', label: t('gettingStarted.step2'), done: hasClient, to: '/clients' },
    { key: 'share', label: t('gettingStarted.step3'), done: false, to: '/clients' },
  ];
  const current = steps.findIndex((s) => !s.done);
  return (
    <div className="getting-started">
      <div className="getting-started-title" dir="auto">
        {t('gettingStarted.title')}
      </div>
      <ol className="getting-started-steps">
        {steps.map((s, i) => (
          <li
            key={s.key}
            className="getting-started-step"
            data-state={s.done ? 'done' : i === current ? 'current' : 'todo'}
          >
            <span className="getting-started-mark">{s.done ? <CheckOutlined /> : i + 1}</span>
            {i === current ? (
              <Link to={s.to} dir="auto">
                {s.label}
              </Link>
            ) : (
              <span dir="auto">{s.label}</span>
            )}
          </li>
        ))}
      </ol>
    </div>
  );
}
