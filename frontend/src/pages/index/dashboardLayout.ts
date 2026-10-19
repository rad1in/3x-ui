import type { DashboardLayout } from '@/generated/zod';

export const GRID_COLUMNS = 12;
export const SPAN_CHOICES = [4, 6, 8, 12] as const;

export type WidgetId =
  | 'vitals'
  | 'traffic'
  | 'clients'
  | 'geo'
  | 'protocols'
  | 'health'
  | 'sharedAccounts'
  | 'system';

export interface WidgetState {
  id: WidgetId;
  hidden: boolean;
  span: number;
}

export const DEFAULT_WIDGETS: readonly WidgetState[] = [
  { id: 'vitals', hidden: false, span: 12 },
  { id: 'traffic', hidden: false, span: 12 },
  { id: 'clients', hidden: false, span: 12 },
  { id: 'geo', hidden: false, span: 12 },
  { id: 'protocols', hidden: false, span: 12 },
  { id: 'health', hidden: false, span: 12 },
  { id: 'sharedAccounts', hidden: false, span: 12 },
  { id: 'system', hidden: false, span: 12 },
];

const clampSpan = (span: number) => Math.min(GRID_COLUMNS, Math.max(1, Math.round(span)));

// Saved entries win for order, size and visibility; widgets the saved layout does
// not know (new releases) keep their default slot at the end, and removed ones drop.
export function resolveLayout(saved: DashboardLayout | null | undefined): WidgetState[] {
  const defaults = new Map(DEFAULT_WIDGETS.map((w) => [w.id as string, w]));
  const out: WidgetState[] = [];
  for (const entry of saved?.widgets ?? []) {
    const base = defaults.get(entry.id);
    if (!base) continue;
    defaults.delete(entry.id);
    out.push({ id: base.id, hidden: entry.hidden, span: clampSpan(entry.span) });
  }
  for (const rest of defaults.values()) out.push({ ...rest });
  return out;
}

export function moveWidget(widgets: WidgetState[], id: WidgetId, toIndex: number): WidgetState[] {
  const from = widgets.findIndex((w) => w.id === id);
  if (from < 0) return widgets;
  const to = Math.min(widgets.length - 1, Math.max(0, toIndex));
  if (to === from) return widgets;
  const next = widgets.slice();
  const [item] = next.splice(from, 1);
  next.splice(to, 0, item);
  return next;
}

export function moveBy(widgets: WidgetState[], id: WidgetId, delta: number): WidgetState[] {
  const from = widgets.findIndex((w) => w.id === id);
  return from < 0 ? widgets : moveWidget(widgets, id, from + delta);
}

export function setSpan(widgets: WidgetState[], id: WidgetId, span: number): WidgetState[] {
  return widgets.map((w) => (w.id === id ? { ...w, span: clampSpan(span) } : w));
}

export function toggleHidden(widgets: WidgetState[], id: WidgetId): WidgetState[] {
  return widgets.map((w) => (w.id === id ? { ...w, hidden: !w.hidden } : w));
}

export function toSaved(widgets: WidgetState[]): DashboardLayout {
  return { widgets: widgets.map((w) => ({ id: w.id, hidden: w.hidden, span: w.span })) };
}
