export interface DailyUsage {
  day: string;
  up: number;
  down: number;
}

function dayKey(d: Date): string {
  const mm = String(d.getMonth() + 1).padStart(2, '0');
  const dd = String(d.getDate()).padStart(2, '0');
  return `${d.getFullYear()}-${mm}-${dd}`;
}

// The server only stores days that had traffic; the chart needs every day.
export function fillDailyUsage(rows: DailyUsage[], days: number, today: Date): DailyUsage[] {
  const byDay = new Map(rows.map((r) => [r.day, r]));
  const out: DailyUsage[] = [];
  for (let offset = days - 1; offset >= 0; offset--) {
    const d = new Date(today.getFullYear(), today.getMonth(), today.getDate() - offset);
    const key = dayKey(d);
    const row = byDay.get(key);
    out.push({ day: key, up: row?.up ?? 0, down: row?.down ?? 0 });
  }
  return out;
}
