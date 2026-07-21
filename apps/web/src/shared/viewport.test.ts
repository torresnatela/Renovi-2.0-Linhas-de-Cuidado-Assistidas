import { act, renderHook } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';

import { useIsDesktop } from './viewport';
import { mockViewport } from './viewport.testkit';
import type { ViewportHandle } from './viewport.testkit';

describe('useIsDesktop', () => {
  let handle: ViewportHandle | undefined;

  afterEach(() => {
    handle?.restore();
    handle = undefined;
  });

  it('sem matchMedia (jsdom puro) assume desktop por padrão', () => {
    const { result } = renderHook(() => useIsDesktop());
    expect(result.current).toBe(true);
  });

  it('com mockViewport("mobile") retorna false', () => {
    handle = mockViewport('mobile');
    const { result } = renderHook(() => useIsDesktop());
    expect(result.current).toBe(false);
  });

  it('com mockViewport("desktop") retorna true', () => {
    handle = mockViewport('desktop');
    const { result } = renderHook(() => useIsDesktop());
    expect(result.current).toBe(true);
  });

  it('handle.set re-renderiza o hook quando o viewport muda', () => {
    handle = mockViewport('desktop');
    const { result } = renderHook(() => useIsDesktop());
    expect(result.current).toBe(true);

    act(() => {
      handle?.set('mobile');
    });

    expect(result.current).toBe(false);
  });

  it('restore() devolve o ambiente sem matchMedia', () => {
    handle = mockViewport('mobile');
    expect(typeof window.matchMedia).toBe('function');

    handle.restore();
    handle = undefined;

    expect(typeof window.matchMedia).not.toBe('function');
  });
});
