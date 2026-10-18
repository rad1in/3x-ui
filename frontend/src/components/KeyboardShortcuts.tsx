import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import { Modal } from 'antd';

import { GO_TO_ROUTES, createShortcutHandler, findSearchInput } from '@/lib/shortcuts';

export default function KeyboardShortcuts() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [helpOpen, setHelpOpen] = useState(false);

  useEffect(() => {
    const handle = createShortcutHandler({
      navigate,
      focusSearch: () => findSearchInput(document)?.focus(),
      openHelp: () => setHelpOpen(true),
    });
    const onKeyDown = (e: KeyboardEvent) => {
      if (handle(e)) e.preventDefault();
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [navigate]);

  const rows = useMemo(
    () => [
      { keys: '/', text: t('shortcuts.focusSearch') },
      { keys: '?', text: t('shortcuts.openHelp') },
      { keys: 'Ctrl/⌘ + K', text: t('shortcuts.openPalette') },
      ...Object.entries(GO_TO_ROUTES).map(([k, r]) => ({
        keys: `g ${k}`,
        text: `${t('shortcuts.goTo')}: ${t(r.label)}`,
      })),
    ],
    [t],
  );

  return (
    <Modal
      open={helpOpen}
      title={t('shortcuts.title')}
      footer={null}
      onCancel={() => setHelpOpen(false)}
    >
      <table data-testid="shortcuts-help" style={{ width: '100%' }}>
        <tbody>
          {rows.map((r) => (
            <tr key={r.keys}>
              <td style={{ padding: '4px 12px 4px 0', whiteSpace: 'nowrap' }}>
                <kbd>{r.keys}</kbd>
              </td>
              <td>{r.text}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </Modal>
  );
}
