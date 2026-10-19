import { describe, expect, it } from 'vitest';

import {
  DEFAULT_WIDGETS,
  moveBy,
  moveWidget,
  resolveLayout,
  setSpan,
  toSaved,
  toggleHidden,
} from '@/pages/index/dashboardLayout';

const ids = (ws: readonly { id: string }[]) => ws.map((w) => w.id);

describe('resolveLayout', () => {
  it('returns the defaults when nothing was saved', () => {
    expect(resolveLayout(null)).toEqual(DEFAULT_WIDGETS);
    expect(resolveLayout({ widgets: [] })).toEqual(DEFAULT_WIDGETS);
  });

  it('restores saved order, size and visibility, then appends widgets the save predates', () => {
    const out = resolveLayout({
      widgets: [
        { id: 'geo', hidden: false, span: 6 },
        { id: 'vitals', hidden: true, span: 12 },
      ],
    });
    expect(ids(out).slice(0, 2)).toEqual(['geo', 'vitals']);
    expect(out[0]).toEqual({ id: 'geo', hidden: false, span: 6 });
    expect(out[1].hidden).toBe(true);
    expect(out).toHaveLength(DEFAULT_WIDGETS.length);
    expect(ids(out).slice(2)).toEqual(
      ids(DEFAULT_WIDGETS).filter((i) => i !== 'geo' && i !== 'vitals'),
    );
  });

  it('drops widgets that no longer exist and clamps a wild span', () => {
    const out = resolveLayout({
      widgets: [
        { id: 'gone', hidden: false, span: 6 },
        { id: 'health', hidden: false, span: 99 },
      ],
    });
    expect(ids(out)).not.toContain('gone');
    expect(out[0]).toEqual({ id: 'health', hidden: false, span: 12 });
  });
});

describe('edit operations', () => {
  const base = resolveLayout(null);

  it('moveWidget places a widget at the target index', () => {
    expect(ids(moveWidget(base, 'system', 0))[0]).toBe('system');
    expect(ids(moveWidget(base, 'vitals', 2)).slice(0, 3)).toEqual([
      'traffic',
      'clients',
      'vitals',
    ]);
  });

  it('moveBy stops at both ends instead of wrapping', () => {
    expect(moveBy(base, 'vitals', -1)).toBe(base);
    expect(moveBy(base, 'system', 1)).toBe(base);
    expect(ids(moveBy(base, 'traffic', 1)).slice(0, 3)).toEqual(['vitals', 'clients', 'traffic']);
  });

  it('setSpan and toggleHidden touch only the target widget', () => {
    const resized = setSpan(base, 'geo', 6);
    expect(resized.find((w) => w.id === 'geo')?.span).toBe(6);
    expect(resized.filter((w) => w.span !== 12)).toHaveLength(1);
    const hidden = toggleHidden(base, 'geo');
    expect(hidden.filter((w) => w.hidden).map((w) => w.id)).toEqual(['geo']);
  });

  it('survives a save and restore round trip', () => {
    const edited = toggleHidden(setSpan(moveWidget(base, 'geo', 0), 'geo', 8), 'health');
    expect(resolveLayout(toSaved(edited))).toEqual(edited);
  });
});
