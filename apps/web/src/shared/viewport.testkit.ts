/**
 * Testkit do viewport: instala um `window.matchMedia` fake nos testes que
 * precisam forçar `useIsDesktop()` para mobile ou desktop (o jsdom do projeto
 * não implementa `matchMedia` — ver domínio em `viewport.ts`).
 *
 * Cada chamada a `mockViewport` cria uma função `matchMedia` NOVA (referência
 * distinta da chamada anterior): é essa troca de referência que faz o cache
 * do hook em `viewport.ts` recalcular a MQL em vez de reaproveitar uma de um
 * teste anterior — por isso `restore()`/nova `mockViewport()` não vazam
 * estado entre `it()`s do mesmo arquivo.
 */
type ViewportMode = 'mobile' | 'desktop';

export interface ViewportHandle {
  set(mode: ViewportMode): void;
  restore(): void;
}

const DESKTOP_QUERY = '(min-width: 1024px)';

export function mockViewport(mode: ViewportMode): ViewportHandle {
  const original = window.matchMedia;
  const listeners = new Set<(event: MediaQueryListEvent) => void>();
  let matches = mode === 'desktop';

  const mql = {
    media: DESKTOP_QUERY,
    get matches() {
      return matches;
    },
    addEventListener(type: string, listener: (event: MediaQueryListEvent) => void) {
      if (type === 'change') listeners.add(listener);
    },
    removeEventListener(_type: string, listener: (event: MediaQueryListEvent) => void) {
      listeners.delete(listener);
    },
  } as unknown as MediaQueryList;

  window.matchMedia = ((query: string) => {
    if (query !== DESKTOP_QUERY) {
      throw new Error(`viewport.testkit: query inesperada "${query}"`);
    }
    return mql;
  }) as typeof window.matchMedia;

  return {
    set(nextMode: ViewportMode) {
      matches = nextMode === 'desktop';
      const event = { matches } as MediaQueryListEvent;
      listeners.forEach((listener) => listener(event));
    },
    restore() {
      window.matchMedia = original;
    },
  };
}
