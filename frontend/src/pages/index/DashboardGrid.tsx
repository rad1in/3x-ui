import { useState } from 'react';
import type { DragEvent, ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Segmented, Space, Tooltip } from 'antd';
import {
  ArrowDownOutlined,
  ArrowUpOutlined,
  EyeInvisibleOutlined,
  EyeOutlined,
  HolderOutlined,
} from '@ant-design/icons';

import {
  SPAN_CHOICES,
  moveBy,
  moveWidget,
  setSpan,
  toggleHidden,
  type WidgetId,
  type WidgetState,
} from './dashboardLayout';

interface DashboardGridProps {
  widgets: WidgetState[];
  editing: boolean;
  content: Partial<Record<WidgetId, ReactNode>>;
  onChange: (edit: (current: WidgetState[]) => WidgetState[]) => void;
}

function useWidgetTitle() {
  const { t } = useTranslation();
  return (id: WidgetId): string => {
    switch (id) {
      case 'vitals':
        return t('pages.index.widgetVitals');
      case 'traffic':
        return t('pages.index.widgetTraffic');
      case 'clients':
        return t('pages.index.widgetClients');
      case 'geo':
        return t('pages.index.clientCountries');
      case 'protocols':
        return t('pages.index.widgetProtocols');
      case 'health':
        return t('pages.index.widgetHealth');
      case 'sharedAccounts':
        return t('pages.index.widgetSharedAccounts');
      case 'system':
        return t('pages.index.widgetSystem');
    }
  };
}

export default function DashboardGrid({ widgets, editing, content, onChange }: DashboardGridProps) {
  const { t } = useTranslation();
  const title = useWidgetTitle();
  const [dragId, setDragId] = useState<WidgetId | null>(null);
  const [overId, setOverId] = useState<WidgetId | null>(null);

  const spanLabel = (span: number) => {
    if (span === 4) return '1/3';
    if (span === 6) return '1/2';
    if (span === 8) return '2/3';
    return t('pages.index.widgetSizeFull');
  };

  const visible = widgets.filter((w) => content[w.id] != null && (editing || !w.hidden));

  function onDrop(e: DragEvent, target: WidgetId) {
    e.preventDefault();
    if (dragId && dragId !== target) {
      const to = widgets.findIndex((w) => w.id === target);
      onChange((cur) => moveWidget(cur, dragId, to));
    }
    setDragId(null);
    setOverId(null);
  }

  return (
    <div className="ov-grid">
      {visible.map((w) => {
        const name = title(w.id);
        return (
          <div
            role="presentation"
            key={w.id}
            className={[
              'ov-widget',
              editing ? 'is-editing' : '',
              w.hidden ? 'is-hidden' : '',
              overId === w.id && dragId !== w.id ? 'is-drop-target' : '',
            ]
              .filter(Boolean)
              .join(' ')}
            style={{ ['--ov-span' as string]: w.span }}
            data-widget={w.id}
            draggable={editing}
            onDragStart={(e) => {
              e.dataTransfer.effectAllowed = 'move';
              e.dataTransfer.setData('text/plain', w.id);
              setDragId(w.id);
            }}
            onDragOver={(e) => {
              if (!dragId) return;
              e.preventDefault();
              setOverId(w.id);
            }}
            onDrop={(e) => onDrop(e, w.id)}
            onDragEnd={() => {
              setDragId(null);
              setOverId(null);
            }}
          >
            {editing && (
              <div className="ov-widget-bar">
                <span className="ov-widget-grip" aria-hidden>
                  <HolderOutlined />
                </span>
                <span className="ov-widget-name">{name}</span>
                <Space size={4} wrap>
                  <Segmented
                    size="small"
                    aria-label={t('pages.index.widgetSize')}
                    value={w.span}
                    options={SPAN_CHOICES.map((s) => ({ value: s, label: spanLabel(s) }))}
                    onChange={(v) => onChange((cur) => setSpan(cur, w.id, Number(v)))}
                  />
                  <Tooltip title={t('pages.index.widgetMoveEarlier')}>
                    <Button
                      size="small"
                      icon={<ArrowUpOutlined />}
                      aria-label={`${t('pages.index.widgetMoveEarlier')}: ${name}`}
                      onClick={() => onChange((cur) => moveBy(cur, w.id, -1))}
                    />
                  </Tooltip>
                  <Tooltip title={t('pages.index.widgetMoveLater')}>
                    <Button
                      size="small"
                      icon={<ArrowDownOutlined />}
                      aria-label={`${t('pages.index.widgetMoveLater')}: ${name}`}
                      onClick={() => onChange((cur) => moveBy(cur, w.id, 1))}
                    />
                  </Tooltip>
                  <Tooltip
                    title={w.hidden ? t('pages.index.widgetShow') : t('pages.index.widgetHide')}
                  >
                    <Button
                      size="small"
                      icon={w.hidden ? <EyeInvisibleOutlined /> : <EyeOutlined />}
                      aria-pressed={w.hidden}
                      aria-label={`${w.hidden ? t('pages.index.widgetShow') : t('pages.index.widgetHide')}: ${name}`}
                      onClick={() => onChange((cur) => toggleHidden(cur, w.id))}
                    />
                  </Tooltip>
                </Space>
              </div>
            )}
            <div className="ov-widget-body">{content[w.id]}</div>
          </div>
        );
      })}
    </div>
  );
}
