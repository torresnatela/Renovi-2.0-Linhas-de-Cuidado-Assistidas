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
  features/<feat>/   # cada tela/feature (auth/, home/, health/, scheduling/, journey/)
  shared/            # api client, helpers, componentes compartilhados
  setupTests.ts      # matchers do Testing Library
```

## Convenções

- **Uma pasta por feature** em `src/features/` (colocation: componente + hooks + teste juntos).
- **Dados da API sempre via TanStack Query** (`useQuery`/`useMutation`), nunca `fetch` solto num componente.
- No MVP, o cliente HTTP manual (`shared/api.ts`) é **substituído pelos hooks gerados
  pelo orval** a partir do OpenAPI (`packages/api-client`). Não escreva tipos de API à mão.
- Tailwind para estilo; sem CSS solto salvo necessidade.
- **Sessão:** cookie `httpOnly` — o JS NÃO o lê. Todo fetch usa `credentials: 'include'`;
  nunca guarde token em `localStorage`. Saber quem está logado = perguntar ao servidor (`useSession`).
- **O cadastro demora de verdade** (é síncrono contra a Doutor ao Vivo, que já levou dezenas
  de segundos). Toda tela que o dispare precisa dizer isso — spinner mudo faz o usuário
  recarregar no meio.
- **Regra de ouro de UX (do motor):** nunca mostre botão só desabilitado — mostre o
  motivo traduzido do `reason` (ex.: "Você já usou sua consulta desta semana").
  A tabela de tradução é `features/scheduling/reasons.ts` — uma só, servindo erro e veredito.
- **Data/hora SEMPRE via `shared/datetime`**, que exige o `timeZone` como parâmetro.
  Nunca `toLocaleTimeString()` solto: ele usa o fuso do BROWSER, e os horários vêm
  do legado como hora de parede de São Paulo — um paciente viajando veria 12:00
  numa consulta das 09:00, sem erro nenhum aparecer.
- **Regra que o servidor calcula, o front NÃO recalcula.** A janela de entrada
  chega como `join.opens_at`, já pronto: "30 minutos" não existe no front (ADR-017).
  Vale para tudo o que vier do motor de elegibilidade também.
- **`Idempotency-Key` do agendamento nasce com a INTENÇÃO** (o clique no horário),
  não com a requisição: o retry do mesmo agendamento reusa a key (replay 200, sem
  consulta duplicada); escolher outro horário gera uma key nova. Ver
  `features/journey/ScheduleCarePage.tsx` (ADR-025).

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
