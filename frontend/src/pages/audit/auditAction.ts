export const AUDIT_LOGIN_ACTION = 'login';

export interface AuditActionParts {
  section: string;
  operation: string;
}

// POST is the panel's default mutation verb, so only other verbs are spelled out.
export function describeAuditAction(action: string): AuditActionParts {
  if (action === AUDIT_LOGIN_ACTION) return { section: AUDIT_LOGIN_ACTION, operation: '' };
  const [method, path = ''] = action.split(' ', 2);
  const segments = path.split('/').filter((s) => s && !s.startsWith(':'));
  const [section = '', ...rest] = segments;
  const op = rest.join(' › ');
  if (!op) return { section, operation: method };
  return { section, operation: method === 'POST' ? op : `${method} ${op}` };
}
