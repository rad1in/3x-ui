export const ADMIN_ROLES = ['owner', 'admin', 'operator', 'viewer'] as const;
export type AdminRole = (typeof ADMIN_ROLES)[number];

const OPERATOR_PAGES = new Set(['/inbounds', '/clients', '/groups', '/sessions', '/reseller']);

// Mirrors the server's role tables so nobody is offered a page that only answers 403.
export function canOpenPage(role: string | undefined, page: string): boolean {
  if (role === 'operator') return OPERATOR_PAGES.has(page);
  if (page === '/admins') return role === 'owner';
  return true;
}

// Where a role lands when it opens the panel root.
export function homePage(role: string | undefined): string {
  return role === 'operator' ? '/clients' : '/';
}
