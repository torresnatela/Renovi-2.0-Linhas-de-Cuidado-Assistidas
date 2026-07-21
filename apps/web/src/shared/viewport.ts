import { useSyncExternalStore } from 'react';

/**
 * ADR-041 — troca de CHROME (estrutura: tab bar vs. sidebar, header mobile vs.
 * desktop etc.) é decidida por este hook; troca só de ESTILO (espaçamento,
 * tamanho de fonte...) continua via classes `lg:` do Tailwind. `DESKTOP_QUERY`
 * espelha esse breakpoint `lg` (1024px) — o único breakpoint estrutural do app.
 *
 * O jsdom usado nos testes NÃO implementa `matchMedia`: por isso o snapshot
 * default, quando `matchMedia` não existe, é `true` (DESKTOP). É esse default
 * que garante que os ~201 testes existentes (escritos antes desta etapa)
 * continuem exercitando o layout desktop sem que nenhum precise ser editado —
 * só telas que chamarem `useIsDesktop()` diretamente precisam simular o
 * viewport via `viewport.testkit`.
 */
const DESKTOP_QUERY = '(min-width: 1024px)';

// Cacheados junto com a referência de `window.matchMedia` que os gerou: em
// produção essa referência é estável (nunca muda durante a vida da página),
// então a MQL é criada uma única vez. Em teste, cada `mockViewport()` instala
// uma função nova — a troca de referência invalida o cache e força recriar a
// MQL, em vez de vazar o estado (e os listeners) de um teste para o outro.
let cachedMatchMedia: typeof window.matchMedia | undefined;
let cachedQuery: MediaQueryList | undefined;

function getMql(): MediaQueryList | undefined {
  const currentMatchMedia = typeof window.matchMedia === 'function' ? window.matchMedia : undefined;
  if (!currentMatchMedia) {
    cachedMatchMedia = undefined;
    cachedQuery = undefined;
    return undefined;
  }
  if (cachedMatchMedia !== currentMatchMedia) {
    cachedMatchMedia = currentMatchMedia;
    cachedQuery = currentMatchMedia(DESKTOP_QUERY);
  }
  return cachedQuery;
}

function getSnapshot(): boolean {
  const mql = getMql();
  return mql ? mql.matches : true;
}

function subscribe(onStoreChange: () => void): () => void {
  const mql = getMql();
  if (!mql) return () => {};
  mql.addEventListener('change', onStoreChange);
  return () => mql.removeEventListener('change', onStoreChange);
}

export function useIsDesktop(): boolean {
  return useSyncExternalStore(subscribe, getSnapshot);
}
