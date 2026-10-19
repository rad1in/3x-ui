import { describe, expect, it, vi } from 'vitest';

import { CHORD_TIMEOUT_MS, createShortcutHandler, type KeyLike } from '@/lib/shortcuts';

function key(k: string, over: Partial<KeyLike> = {}): KeyLike {
  return { key: k, ctrlKey: false, metaKey: false, altKey: false, target: null, ...over };
}

function setup() {
  let t = 1000;
  const actions = { navigate: vi.fn(), focusSearch: vi.fn(), openHelp: vi.fn() };
  const handle = createShortcutHandler(actions, () => t);
  return { actions, handle, advance: (ms: number) => (t += ms) };
}

describe('keyboard shortcuts', () => {
  it('"/" focuses search and "?" opens the help', () => {
    const { actions, handle } = setup();
    expect(handle(key('/'))).toBe(true);
    expect(handle(key('?'))).toBe(true);
    expect(actions.focusSearch).toHaveBeenCalledTimes(1);
    expect(actions.openHelp).toHaveBeenCalledTimes(1);
  });

  it('"g" then a letter navigates, and the chord resets afterwards', () => {
    const { actions, handle } = setup();
    handle(key('g'));
    expect(handle(key('c'))).toBe(true);
    expect(actions.navigate).toHaveBeenCalledWith('/clients');
    expect(handle(key('c'))).toBe(false);
    expect(actions.navigate).toHaveBeenCalledTimes(1);
  });

  it('forgets a pending "g" after the chord timeout', () => {
    const { actions, handle, advance } = setup();
    handle(key('g'));
    advance(CHORD_TIMEOUT_MS + 1);
    expect(handle(key('i'))).toBe(false);
    expect(actions.navigate).not.toHaveBeenCalled();
  });

  it('ignores keys typed into a text field or pressed with a modifier', () => {
    const { actions, handle } = setup();
    const input = { tagName: 'INPUT' } as unknown as EventTarget;
    expect(handle(key('/', { target: input }))).toBe(false);
    expect(handle(key('g', { ctrlKey: true }))).toBe(false);
    expect(handle(key('?', { metaKey: true }))).toBe(false);
    expect(actions.focusSearch).not.toHaveBeenCalled();
    expect(actions.openHelp).not.toHaveBeenCalled();
  });

  it('an unknown second key cancels the chord instead of leaving it armed', () => {
    const { actions, handle } = setup();
    handle(key('g'));
    expect(handle(key('z'))).toBe(false);
    expect(handle(key('i'))).toBe(false);
    expect(actions.navigate).not.toHaveBeenCalled();
  });
});
