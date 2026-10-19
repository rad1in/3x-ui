export const GO_TO_ROUTES: Record<string, { path: string; label: string }> = {
  o: { path: '/', label: 'menu.dashboard' },
  i: { path: '/inbounds', label: 'menu.inbounds' },
  c: { path: '/clients', label: 'menu.clients' },
  g: { path: '/groups', label: 'menu.groups' },
  n: { path: '/nodes', label: 'menu.nodes' },
  h: { path: '/hosts', label: 'menu.hosts' },
  s: { path: '/settings', label: 'menu.settings' },
  x: { path: '/xray', label: 'menu.xray' },
  a: { path: '/audit', label: 'menu.audit' },
};

// A second key must follow the "g" prefix within this long.
export const CHORD_TIMEOUT_MS = 1000;

export interface KeyLike {
  key: string;
  ctrlKey: boolean;
  metaKey: boolean;
  altKey: boolean;
  target: EventTarget | null;
}

export interface ShortcutActions {
  navigate: (path: string) => void;
  focusSearch: () => void;
  openHelp: () => void;
}

export function isTypingTarget(target: EventTarget | null): boolean {
  const el = target as HTMLElement | null;
  if (!el || typeof el.tagName !== 'string') return false;
  const tag = el.tagName;
  return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || el.isContentEditable === true;
}

// Returns a handler that reports whether it consumed the key (caller then calls preventDefault).
export function createShortcutHandler(actions: ShortcutActions, now: () => number = Date.now) {
  let chordStartedAt = 0;
  return function handle(e: KeyLike): boolean {
    if (e.ctrlKey || e.metaKey || e.altKey || isTypingTarget(e.target)) {
      chordStartedAt = 0;
      return false;
    }
    if (chordStartedAt && now() - chordStartedAt <= CHORD_TIMEOUT_MS) {
      chordStartedAt = 0;
      const route = GO_TO_ROUTES[e.key.toLowerCase()];
      if (route) {
        actions.navigate(route.path);
        return true;
      }
    }
    chordStartedAt = 0;
    switch (e.key) {
      case '/':
        actions.focusSearch();
        return true;
      case '?':
        actions.openHelp();
        return true;
      case 'g':
        chordStartedAt = now();
        return true;
      default:
        return false;
    }
  };
}

// The first visible filter or search box of the current page, if any.
export function findSearchInput(root: ParentNode): HTMLInputElement | null {
  const candidates = root.querySelectorAll<HTMLInputElement>(
    'input[type="search"], .filter-bar input, .ant-input-search input',
  );
  for (const el of candidates) {
    if (!el.disabled && el.offsetParent !== null) return el;
  }
  return null;
}
