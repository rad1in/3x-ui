export type ReportFormat = 'csv' | 'xlsx' | 'pdf';

// Query values that are empty are left out; the panel may sit under a base path.
export function reportUrl(
  path: string,
  params: Record<string, string>,
  basePath = window.X_UI_BASE_PATH || '',
): string {
  const query = new URLSearchParams(
    Object.entries(params).filter(([, value]) => value !== ''),
  ).toString();
  return `${basePath}${path}${query ? `?${query}` : ''}`;
}

// The endpoints answer with Content-Disposition: attachment, so navigating saves the file.
export function downloadReport(path: string, params: Record<string, string>): void {
  window.location.href = reportUrl(path, params);
}
