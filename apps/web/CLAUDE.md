# CLAUDE.md — Front (apps/web)

Contexto local do front React. Regras gerais no `CLAUDE.md` da raiz.

## Stack

React 18 · TypeScript · Vite · Tailwind · **TanStack Query** (cache/refetch da
jornada e elegibilidade) · Vitest + Testing Library. Consumo da API via proxy
`/api` (dev) → API Go em `:8090`.

## Estrutura

```
src/
  main.tsx           # bootstrap + QueryClientProvider
  App.tsx            # shell
  features/<feat>/   # cada tela/feature (ex.: health/, e no MVP: journey/, scheduling/)
  shared/            # api client, helpers, componentes compartilhados
  setupTests.ts      # matchers do Testing Library
```

## Convenções

- **Uma pasta por feature** em `src/features/` (colocation: componente + hooks + teste juntos).
- **Dados da API sempre via TanStack Query** (`useQuery`/`useMutation`), nunca `fetch` solto num componente.
- No MVP, o cliente HTTP manual (`shared/api.ts`) é **substituído pelos hooks gerados
  pelo orval** a partir do OpenAPI (`packages/api-client`). Não escreva tipos de API à mão.
- Tailwind para estilo; sem CSS solto salvo necessidade.
- **Regra de ouro de UX (do motor):** nunca mostre botão só desabilitado — mostre o
  motivo traduzido do `reason` (ex.: "Você já usou sua consulta desta semana").

## Comandos

```bash
make web-install   # npm install
make web-dev       # dev server (http://localhost:5173)
make web-test      # vitest
npm --prefix apps/web run build       # build de produção
npm --prefix apps/web run typecheck   # tsc --noEmit
```

## Ao terminar

Atualize `docs/PROGRESSO.md` (telas concluídas) e este arquivo se mudou o padrão de estrutura.
